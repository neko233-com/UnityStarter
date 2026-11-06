package main

import (
	"context"
	"strings"
	"time"
)

type projectVersionWatch struct{ cancel context.CancelFunc }

// An editor may update the project version long after launch, after the user
// accepts its own migration prompt. Do not modify the project ourselves.
func waitForProjectVersion(ctx context.Context, path, previous string, interval time.Duration) bool {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return false
		}
		if version, _, ok := readProject(path); ok && parseEditorProductVersion(version) == version && !strings.EqualFold(version, previous) {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}

func (a *app) watchProjectVersion(path, previous string) {
	if a.win == nil {
		return
	}
	parent := a.updateCtx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 20*time.Minute)
	key := localProjectKey(path)
	if a.projectVersionWatches == nil {
		a.projectVersionWatches = map[string]*projectVersionWatch{}
	}
	if old := a.projectVersionWatches[key]; old != nil {
		old.cancel()
	}
	watch := &projectVersionWatch{cancel: cancel}
	a.projectVersionWatches[key] = watch
	win := a.win
	go func() {
		changed := waitForProjectVersion(ctx, path, previous, 2*time.Second)
		cancel()
		if parent.Err() != nil {
			return
		}
		win.Update(func() {
			if a.projectVersionWatches[key] != watch {
				return
			}
			delete(a.projectVersionWatches, key)
			if changed {
				a.refresh()
			}
		})
	}()
}
