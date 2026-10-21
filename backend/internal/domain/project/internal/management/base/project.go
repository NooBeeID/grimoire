package base

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	NameMinRunes        = 2
	NameMaxRunes        = 100
	DescriptionMaxRunes = 1000
)

type Project struct {
	ID             string
	Name           string
	Description    string
	CreatedBy      *string
	CreatedByLabel *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ValidateName men-trim dan memvalidasi nama; FieldErrors nil bila valid.
func ValidateName(s string) (string, FieldErrors) {
	s = strings.TrimSpace(s)
	n := utf8.RuneCountInString(s)
	if n < NameMinRunes || n > NameMaxRunes {
		return s, FieldErrors{"name": "nama project wajib 2-100 karakter"}
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return s, FieldErrors{"name": "nama project tidak boleh mengandung karakter kontrol"}
		}
	}
	return s, nil
}

// ValidateDescription men-trim dan memvalidasi deskripsi; FieldErrors nil bila valid.
func ValidateDescription(s string) (string, FieldErrors) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > DescriptionMaxRunes {
		return s, FieldErrors{"description": "deskripsi maksimal 1000 karakter"}
	}
	for _, r := range s {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return s, FieldErrors{"description": "deskripsi mengandung karakter tidak valid"}
		}
	}
	return s, nil
}

// NewProject memvalidasi nama dan deskripsi (error digabung). ID diisi DB.
func NewProject(name, description string, actor Actor, now time.Time) (Project, error) {
	n, e1 := ValidateName(name)
	d, e2 := ValidateDescription(description)
	if fe := e1.Merge(e2); len(fe) > 0 {
		return Project{}, InvalidFields(fe)
	}
	id, label := actor.ID, actor.Label
	return Project{
		Name: n, Description: d, CreatedBy: &id, CreatedByLabel: &label,
		CreatedAt: now.UTC(), UpdatedAt: now.UTC(),
	}, nil
}

type ProjectPatch struct{ Name, Description *string }

func (p ProjectPatch) Empty() bool { return p.Name == nil && p.Description == nil }

// ApplyUpdate menerapkan patch. changed berurutan ["name","description"]; UpdatedAt di-set bila ada perubahan.
// Struct tidak berubah bila ada error validasi.
func (p *Project) ApplyUpdate(patch ProjectPatch, now time.Time) (changed []string, err error) {
	if patch.Empty() {
		return nil, InvalidFields(FieldErrors{"body": "minimal satu field (name, description) wajib diisi"})
	}
	var fe FieldErrors
	newName, newDesc := p.Name, p.Description
	if patch.Name != nil {
		n, e := ValidateName(*patch.Name)
		fe = fe.Merge(e)
		newName = n
	}
	if patch.Description != nil {
		d, e := ValidateDescription(*patch.Description)
		fe = fe.Merge(e)
		newDesc = d
	}
	if len(fe) > 0 {
		return nil, InvalidFields(fe)
	}
	if patch.Name != nil && newName != p.Name {
		changed = append(changed, "name")
	}
	if patch.Description != nil && newDesc != p.Description {
		changed = append(changed, "description")
	}
	if len(changed) == 0 {
		return nil, nil
	}
	p.Name, p.Description = newName, newDesc
	p.UpdatedAt = now.UTC()
	return changed, nil
}

// ConfirmDeletion: confirm harus sama persis dengan nama project.
func (p Project) ConfirmDeletion(confirm string) error {
	if confirm == p.Name {
		return nil
	}
	return ErrConfirmationMismatch
}

func (p Project) ToDTO() ProjectDTO {
	return ProjectDTO{
		ID: p.ID, Name: p.Name, Description: p.Description,
		CreatedBy: p.CreatedBy, CreatedByLabel: p.CreatedByLabel,
		CreatedAt: p.CreatedAt.UTC(), UpdatedAt: p.UpdatedAt.UTC(),
	}
}
