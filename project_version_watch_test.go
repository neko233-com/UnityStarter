package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProjectVersionWatchWaitsForActualMigration(t *testing.T) {
	project := filepath.Join(t.TempDir(), "Game")
	writeProject(t, project, "2022.3.51f1")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result := make(chan bool, 1)
	go func() { result <- waitForProjectVersion(ctx, project, "2022.3.51f1", 10*time.Millisecond) }()
	select {
	case <-result:
		t.Fatal("unchanged project reported a migration")
	case <-time.After(30 * time.Millisecond):
	}
	// A temporarily incomplete version file must not be treated as success.
	file := filepath.Join(project, "ProjectSettings", "ProjectVersion.txt")
	if err := os.WriteFile(file, []byte("m_EditorVersion: 6000.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-result:
		t.Fatal("incomplete version file reported a migration")
	case <-time.After(30 * time.Millisecond):
	}
	writeProject(t, project, "6000.0.1f1")
	select {
	case changed := <-result:
		if !changed {
			t.Fatal("actual version change was missed")
		}
	case <-ctx.Done():
		t.Fatal("version change was not observed")
	}
}

func TestProjectVersionWatchStopsWhenCanceled(t *testing.T) {
	project := filepath.Join(t.TempDir(), "Game")
	writeProject(t, project, "2022.3.51f1")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if waitForProjectVersion(ctx, project, "2022.3.51f1", time.Millisecond) {
		t.Fatal("canceled watch reported a change")
	}
}
