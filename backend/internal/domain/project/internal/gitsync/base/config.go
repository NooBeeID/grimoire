package base

import "time"

// Config adalah konfigurasi sync terakhir per (project, kind).
type Config struct {
	ID, ProjectID                                     string
	Kind                                              Kind
	Provider, Host, RepoURL, RepoPath, Branch, Folder string
	TargetPrefix                                      string
	LastStatus                                        *string
	LastSyncedAt                                      *time.Time
	LastSyncedBy, LastSyncedByLabel                   *string
	LastResult                                        *Report
	CreatedAt, UpdatedAt                              time.Time
}

// NewConfig membangun config dari hasil parse; TargetPrefix dikosongkan untuk api_spec.
func NewConfig(projectID string, kind Kind, ref RepoRef, prefix string) Config {
	if kind != KindKnowledge {
		prefix = ""
	}
	return Config{
		ProjectID: projectID, Kind: kind, Provider: ref.Provider, Host: ref.Host,
		RepoURL: ref.CanonicalURL, RepoPath: ref.Path, Branch: ref.Branch, Folder: ref.Folder, TargetPrefix: prefix,
	}
}

// ToDTO memetakan ke DTO (UTC). LastSyncedBy nil bila belum pernah sync.
func (c Config) ToDTO() ConfigDTO {
	d := ConfigDTO{
		Provider: c.Provider, Host: c.Host, RepoURL: c.RepoURL, RepoPath: c.RepoPath, Branch: c.Branch,
		Folder: c.Folder, TargetPrefix: c.TargetPrefix, LastStatus: c.LastStatus,
	}
	if c.LastSyncedAt != nil {
		t := c.LastSyncedAt.UTC()
		d.LastSyncedAt = &t
	}
	if c.LastSyncedBy != nil {
		label := ""
		if c.LastSyncedByLabel != nil {
			label = *c.LastSyncedByLabel
		}
		d.LastSyncedBy = &SyncedByDTO{ID: *c.LastSyncedBy, Label: label}
	}
	if c.LastResult != nil {
		r := c.LastResult.ToDTO()
		d.LastResult = &r
	}
	return d
}
