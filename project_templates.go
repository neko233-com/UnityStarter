package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type templatePackage struct {
	Name    string `json:"name"`
	ID      string `json:"packageName"`
	Version string `json:"version"`
}

type projectTemplate struct {
	ID          string            `json:"name"`
	Name        string            `json:"displayName"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Category    string            `json:"type"`
	Pipeline    string            `json:"renderPipeline"`
	URL         string            `json:"remoteTar"`
	Integrity   string            `json:"integrity"`
	Size        int64             `json:"size"`
	Packages    []templatePackage `json:"packages"`
	Platforms   []string          `json:"buildPlatforms"`
	TermsURL    string            `json:"termsOfServiceUrl"`
	Archive     string            `json:"-"`
}

func (t projectTemplate) key() string { return t.ID + "@" + t.Version }
func templateEditorKey(e editorInstall) string {
	return e.Kind + "|" + e.Version + "|" + localProjectKey(e.Exe)
}

func templateCachePath(kind string, t projectTemplate) string {
	return filepath.Join(userDataDir(), "templates", kind, t.ID+"-"+t.Version+".tgz")
}

func templateCatalogPath(e editorInstall) string {
	sum := sha256.Sum256([]byte(e.Kind + "|" + e.Version))
	return filepath.Join(userDataDir(), "template-catalogs", hex.EncodeToString(sum[:])+".json")
}

func validTemplateID(kind, id string) bool {
	if !moduleIDPattern.MatchString(id) {
		return false
	}
	if kind == kindTuanjie {
		return strings.HasPrefix(id, "cn.tuanjie.")
	}
	return strings.HasPrefix(id, "com.unity.")
}

func templateURLAllowed(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Host != u.Hostname() || u.RawQuery != "" || u.Fragment != "" || !strings.HasSuffix(u.Path, ".tgz") {
		return false
	}
	switch u.Hostname() {
	case "download.packages.unity.com", "download-packages.cdn.unity.cn", "download-packages.unity.cn", "packages.tuanjie.cn", "packages.unity.com", "packages.unity.cn":
		return true
	}
	return false
}

// Hub catalogs also use base64(hex(sha1)), whereas UPM's shasum is hex.
// Normalize here without changing installer-integrity behavior elsewhere.
func templateIntegrity(value string) (string, error) {
	algorithm, encoded, ok := strings.Cut(value, "-")
	if !ok {
		return "", fmt.Errorf("template checksum is missing")
	}
	sizes := map[string]int{"sha1": 20, "sha256": 32, "sha384": 48}
	size := sizes[algorithm]
	if size == 0 {
		return "", fmt.Errorf("unsupported template checksum")
	}
	digest, err := base64.StdEncoding.DecodeString(encoded)
	if err == nil && len(digest) == size*2 {
		digest, err = hex.DecodeString(string(digest))
	}
	if err != nil || len(digest) != size {
		return "", fmt.Errorf("invalid template checksum")
	}
	return algorithm + "-" + base64.StdEncoding.EncodeToString(digest), nil
}

func validateRemoteTemplate(kind string, t *projectTemplate) error {
	if !validTemplateID(kind, t.ID) || !moduleIDPattern.MatchString(t.Version) || t.Name == "" || !templateURLAllowed(t.URL) || t.Size < 0 {
		return fmt.Errorf("invalid official template")
	}
	integrity, err := templateIntegrity(t.Integrity)
	if err != nil {
		return err
	}
	t.Integrity = integrity
	if t.Category == "" {
		t.Category = "CORE"
	}
	return nil
}

func readTemplateArchive(file, kind string) (projectTemplate, error) {
	var result projectTemplate
	info, err := os.Stat(file)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4<<30 {
		return result, fmt.Errorf("template archive is missing or too large")
	}
	f, err := os.Open(file)
	if err != nil {
		return result, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return result, err
	}
	defer gz.Close()
	tr := tar.NewReader(io.LimitReader(gz, 16<<30))
	for n := 0; n < 200000; n++ {
		h, err := tr.Next()
		if err != nil {
			return result, fmt.Errorf("template package metadata is missing: %w", err)
		}
		if strings.TrimPrefix(h.Name, "./") != "package/package.json" {
			continue
		}
		if h.Typeflag != tar.TypeReg || h.Size > 2<<20 {
			return result, fmt.Errorf("invalid template metadata")
		}
		var meta struct {
			Name         string            `json:"name"`
			DisplayName  string            `json:"displayName"`
			Version      string            `json:"version"`
			Type         string            `json:"type"`
			Description  string            `json:"description"`
			Dependencies map[string]string `json:"dependencies"`
		}
		if err := json.NewDecoder(io.LimitReader(tr, 2<<20)).Decode(&meta); err != nil {
			return result, err
		}
		if meta.Type != "template" || !validTemplateID(kind, meta.Name) || !moduleIDPattern.MatchString(meta.Version) {
			return result, fmt.Errorf("invalid template identity")
		}
		result = projectTemplate{ID: meta.Name, Name: meta.DisplayName, Version: meta.Version, Description: meta.Description, Category: "CORE", Archive: file, Size: info.Size(), Pipeline: "BUILT_IN"}
		if result.Name == "" {
			result.Name = result.ID
		}
		for id, version := range meta.Dependencies {
			result.Packages = append(result.Packages, templatePackage{ID: id, Name: id, Version: version})
			if id == "com.unity.render-pipelines.universal" {
				result.Pipeline = "URP"
			}
			if id == "com.unity.render-pipelines.high-definition" {
				result.Pipeline = "HDRP"
			}
		}
		sort.Slice(result.Packages, func(i, j int) bool { return result.Packages[i].ID < result.Packages[j].ID })
		return result, nil
	}
	return result, fmt.Errorf("template archive has too many entries")
}

func localProjectTemplates(e editorInstall) []projectTemplate {
	dir := filepath.Join(filepath.Dir(e.Exe), "Data", "Resources", "PackageManager", "ProjectTemplates")
	var files []string
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".tgz") {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	// Only the selected Editor's explicit Hub dependencies are eligible. A
	// cached template for another Editor must not silently become compatible.
	if base := os.Getenv("APPDATA"); base != "" {
		hubName := "UnityHub"
		if e.Kind == kindTuanjie {
			hubName = "TuanjieHub"
		}
		dir := filepath.Join(base, hubName, "Templates")
		data, _ := os.ReadFile(filepath.Join(dir, "manifest.json"))
		var manifest map[string]struct {
			Dependencies map[string]string `json:"dependencies"`
		}
		if len(data) < 4<<20 && json.Unmarshal(data, &manifest) == nil {
			for id, version := range manifest[e.Version].Dependencies {
				if validTemplateID(e.Kind, id) && moduleIDPattern.MatchString(version) {
					files = append(files, filepath.Join(dir, id+"-"+version+".tgz"))
				}
			}
		}
	}
	var templates []projectTemplate
	for _, file := range files {
		if t, err := readTemplateArchive(file, e.Kind); err == nil {
			templates = append(templates, t)
		}
	}
	return templates
}

func mergeProjectTemplates(e editorInstall, local, remote []projectTemplate) []projectTemplate {
	byKey := map[string]projectTemplate{}
	for _, t := range local {
		byKey[t.key()] = t
	}
	for _, t := range remote {
		if old, ok := byKey[t.key()]; ok {
			t.Archive = old.Archive
		} else {
			file := templateCachePath(e.Kind, t)
			if info, err := os.Stat(file); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
				t.Archive = file
			}
		}
		byKey[t.key()] = t
	}
	var result []projectTemplate
	for _, t := range byKey {
		result = append(result, t)
	}
	sort.Slice(result, func(i, j int) bool {
		category := func(s string) int {
			switch s {
			case "CORE":
				return 0
			case "SAMPLE":
				return 1
			default:
				return 2
			}
		}
		if x, y := category(result[i].Category), category(result[j].Category); x != y {
			return x < y
		}
		priority := func(t projectTemplate) int {
			if strings.HasSuffix(t.ID, ".urp-blank") || strings.HasSuffix(t.ID, ".universal") {
				return 0
			}
			if t.Pipeline == "URP" {
				return 1
			}
			return 2
		}
		if x, y := priority(result[i]), priority(result[j]); x != y {
			return x < y
		}
		if result[i].Pipeline == "URP" && result[j].Pipeline != "URP" {
			return true
		}
		if result[i].Pipeline != "URP" && result[j].Pipeline == "URP" {
			return false
		}
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return compareVersions(result[i].Version, result[j].Version) > 0
	})
	return result
}

func cachedProjectTemplates(e editorInstall) []projectTemplate {
	info, statErr := os.Stat(templateCatalogPath(e))
	if statErr != nil || !info.Mode().IsRegular() || info.Size() > 16<<20 {
		return nil
	}
	data, err := os.ReadFile(templateCatalogPath(e))
	if err != nil || len(data) > 16<<20 {
		return nil
	}
	var items []projectTemplate
	if json.Unmarshal(data, &items) != nil || len(items) > 512 {
		return nil
	}
	for i := range items {
		if validateRemoteTemplate(e.Kind, &items[i]) != nil {
			return nil
		}
	}
	return items
}

var templateGraphURL = "https://live-platform-api.prd.ld.unity3d.com/graphql"
var templateChinaBase = "https://public-cdn.cloud.unitychina.cn/hub/prod/template/"
var templateTuanjieBase = "https://public-cdn.tuanjie.cn/hub/prod/templates/templates-"

type templateNode struct {
	Name        string   `json:"name"`
	ID          string   `json:"packageName"`
	Description string   `json:"description"`
	Category    string   `json:"type"`
	Pipeline    string   `json:"renderPipeline"`
	Platforms   []string `json:"buildPlatforms"`
	Terms       *struct {
		URL string `json:"url"`
	} `json:"termsOfService"`
	Versions []struct {
		Name string `json:"name"`
		Size struct {
			Value int64 `json:"value"`
		} `json:"size"`
		Tarball struct {
			URL       string `json:"url"`
			Integrity string `json:"integrity"`
		} `json:"tarball"`
		Packages []templatePackage `json:"dependencies"`
	} `json:"versions"`
}

type templateResponse struct {
	Data struct {
		Templates struct {
			Edges []struct {
				Node templateNode `json:"node"`
			} `json:"edges"`
		} `json:"getTemplates"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func templateRequest(ctx context.Context, method, endpoint string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", downloadUserAgent)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("template catalog: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 16<<20 {
		return nil, fmt.Errorf("template catalog is too large")
	}
	return data, nil
}

func decodeTemplateNodes(data []byte) ([]projectTemplate, error) {
	var res templateResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	if len(res.Errors) > 0 {
		return nil, fmt.Errorf("template catalog: %s", res.Errors[0].Message)
	}
	var templates []projectTemplate
	for _, edge := range res.Data.Templates.Edges {
		n := edge.Node
		if len(n.Versions) == 0 {
			continue
		}
		v := n.Versions[0]
		t := projectTemplate{ID: n.ID, Name: n.Name, Version: v.Name, Description: n.Description, Category: n.Category, Pipeline: n.Pipeline, URL: v.Tarball.URL, Integrity: v.Tarball.Integrity, Size: v.Size.Value, Packages: v.Packages, Platforms: n.Platforms}
		if n.Terms != nil {
			t.TermsURL = n.Terms.URL
		}
		if validateRemoteTemplate(kindUnity, &t) == nil {
			templates = append(templates, t)
		}
	}
	return templates, nil
}

func fetchProjectTemplates(ctx context.Context, e editorInstall) ([]projectTemplate, error) {
	if !moduleIDPattern.MatchString(e.Version) {
		return nil, fmt.Errorf("invalid editor version")
	}
	if e.Kind == kindTuanjie {
		data, err := templateRequest(ctx, http.MethodGet, templateTuanjieBase+e.Version+".json", nil)
		if err != nil {
			return nil, err
		}
		var byVersion map[string][]projectTemplate
		if err := json.Unmarshal(data, &byVersion); err != nil {
			return nil, err
		}
		var items []projectTemplate
		for _, t := range byVersion[e.Version] {
			if validateRemoteTemplate(e.Kind, &t) == nil {
				items = append(items, t)
			}
		}
		return items, nil
	}
	parts := strings.Split(e.Version, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid editor version")
	}
	branch := parts[0] + "." + parts[1]
	// The official regional CDN avoids a global request for released branches.
	if data, err := templateRequest(ctx, http.MethodGet, templateChinaBase+branch+".json", nil); err == nil {
		if items, err := decodeTemplateNodes(data); err == nil && len(items) > 0 {
			return items, nil
		}
	}
	const query = `query Templates($limit:Int!,$skip:Int!,$orderBy:TemplateOrder!,$supportedUnityEditorVersions:[String!]!){getTemplates(limit:$limit,skip:$skip,orderBy:$orderBy,supportedUnityEditorVersions:$supportedUnityEditorVersions){edges{node{name packageName description type renderPipeline buildPlatforms termsOfService{url} versions{name size{value} tarball{url integrity} dependencies{name packageName version}}}}}}`
	var result []projectTemplate
	for skip := 0; skip < 500; skip += 100 {
		body, _ := json.Marshal(map[string]any{"query": query, "variables": map[string]any{"limit": 100, "skip": skip, "orderBy": "WEIGHTED_DESC", "supportedUnityEditorVersions": []string{branch}}})
		data, err := templateRequest(ctx, http.MethodPost, templateGraphURL, body)
		if err != nil {
			return nil, err
		}
		items, err := decodeTemplateNodes(data)
		if err != nil {
			return nil, err
		}
		result = append(result, items...)
		var response templateResponse
		_ = json.Unmarshal(data, &response)
		if len(response.Data.Templates.Edges) < 100 {
			return result, nil
		}
	}
	return nil, fmt.Errorf("template catalog pagination exceeded")
}

func verifyProjectTemplate(t projectTemplate, kind string) error {
	actual, err := readTemplateArchive(t.Archive, kind)
	if err != nil {
		return err
	}
	if actual.ID != t.ID || actual.Version != t.Version {
		return fmt.Errorf("template identity changed; refresh templates")
	}
	// Bundled Editor archives can have a different official package envelope.
	// Only our own downloaded cache is tied to the remote tarball's digest.
	if strings.EqualFold(t.Archive, templateCachePath(kind, t)) {
		integrity, err := templateIntegrity(t.Integrity)
		if err != nil {
			return err
		}
		h, expected, err := integrityHasher(integrity)
		if err != nil {
			return err
		}
		f, err := os.Open(t.Archive)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := io.Copy(h, f); err != nil {
			return err
		}
		return verifyIntegrity(h, expected)
	}
	return nil
}
