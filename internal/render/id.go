package render

// ShortID returns the first 8 characters of id when short is true and id
// is long enough; otherwise returns id unchanged. Used by interactive list
// rendering. No truncation marker is emitted because short IDs are
// canonical interactive form (the local cache resolves them back to full
// IDs on paste).
func ShortID(id string, short bool) string {
	if !short || len(id) <= 8 {
		return id
	}
	return id[:8]
}
