package router

import "github.com/ibrhajjaj/ig-dl/internal/types"

// SavedTarget returns the Target representing the authenticated user's
// Saved collection. The CLI's `ig-dl saved` command uses this to dispatch
// without needing a URL or handle — the session owner's own saved items
// are fetched by the backend.
func SavedTarget() types.Target {
	return types.Target{Kind: types.TargetSaved}
}
