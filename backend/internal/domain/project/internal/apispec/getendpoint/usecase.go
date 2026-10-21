// Package getendpoint menyediakan handler bus project.apispec.get-endpoint (provider T06, consumer T09).
package getendpoint

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/openapi"
	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/project/apispec/getendpoint"
)

const maxSimilar = 5

type repository interface {
	ListSpecNames(ctx context.Context, projectID string) ([]string, error)
	FindSpec(ctx context.Context, projectID, name string) (base.Spec, bool, error)
	ListSpecEndpoints(ctx context.Context, specID string) ([]base.Endpoint, error)
}

type Usecase struct {
	repo repository
	log  *slog.Logger
}

func NewUsecase(repo repository, log *slog.Logger) *Usecase {
	if log == nil {
		log = slog.Default()
	}
	return &Usecase{repo: repo, log: log}
}

type params struct {
	projectID, spec, method, path, operationID string
	depth, maxBytes                            int
}

func invalid(msg string) error { return errs.Wrap(errs.CodeInvalid, msg, errs.ErrInvalid) }

func normalize(req getendpoint.Request) (params, error) {
	p := params{
		projectID:   req.ProjectID,
		spec:        req.Spec,
		method:      strings.ToUpper(strings.TrimSpace(req.Method)),
		path:        strings.TrimSpace(req.Path),
		operationID: strings.TrimSpace(req.OperationID),
		depth:       req.SchemaDepth,
		maxBytes:    req.MaxBytes,
	}
	if _, err := uuid.Parse(p.projectID); err != nil {
		return params{}, invalid("project_id tidak valid")
	}
	hasMP := p.method != "" || p.path != ""
	switch {
	case !hasMP && p.operationID == "":
		return params{}, invalid("isi method dan path, atau operation_id")
	case hasMP && p.operationID != "":
		return params{}, invalid("isi method dan path, atau operation_id; tidak boleh keduanya")
	case hasMP && (p.method == "" || p.path == ""):
		return params{}, invalid("method dan path harus diisi bersamaan")
	}
	if hasMP {
		if !base.ValidMethod(p.method) {
			return params{}, invalid("method tidak valid")
		}
		if !strings.HasPrefix(p.path, "/") {
			return params{}, invalid("path harus diawali '/'")
		}
	}
	if p.depth <= 0 {
		p.depth = getendpoint.DefaultSchemaDepth
	} else if p.depth > getendpoint.MaxSchemaDepth {
		p.depth = getendpoint.MaxSchemaDepth
	}
	if p.maxBytes <= 0 {
		p.maxBytes = getendpoint.DefaultMaxBytes
	}
	return p, nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func without(names []string, name string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		if n != name {
			out = append(out, n)
		}
	}
	return out
}

// Execute mengambil satu operasi OpenAPI. Query selalu dibatasi req.ProjectID (isolasi project).
func (u *Usecase) Execute(ctx context.Context, req getendpoint.Request) (getendpoint.Response, error) {
	p, err := normalize(req)
	if err != nil {
		return getendpoint.Response{}, err
	}

	var (
		specName string
		spec     base.Spec
		found    bool
	)
	if p.spec != "" {
		// Jalur umum: satu query.
		spec, found, err = u.repo.FindSpec(ctx, p.projectID, p.spec)
		if err != nil {
			return getendpoint.Response{}, fmt.Errorf("getendpoint: find spec: %w", err)
		}
		specName = p.spec
		if !found {
			names, err := u.repo.ListSpecNames(ctx, p.projectID)
			if err != nil {
				return getendpoint.Response{}, fmt.Errorf("getendpoint: list spec names: %w", err)
			}
			return notFoundSpec(getendpoint.ReasonSpecNotFound, without(names, p.spec)), nil
		}
	} else {
		names, err := u.repo.ListSpecNames(ctx, p.projectID)
		if err != nil {
			return getendpoint.Response{}, fmt.Errorf("getendpoint: list spec names: %w", err)
		}
		if len(names) != 1 {
			return notFoundSpec(getendpoint.ReasonSpecRequired, names), nil
		}
		specName = names[0]
		spec, found, err = u.repo.FindSpec(ctx, p.projectID, specName)
		if err != nil {
			return getendpoint.Response{}, fmt.Errorf("getendpoint: find spec: %w", err)
		}
		if !found {
			return notFoundSpec(getendpoint.ReasonSpecNotFound, without(names, specName)), nil
		}
	}

	doc, issues := openapi.Parse([]byte(spec.Content), openapi.Format(spec.Format), openapi.DefaultLimits)
	if len(issues) > 0 {
		u.log.Error("getendpoint: reparse stored spec gagal",
			"feature", "getendpoint", "project_id", p.projectID, "spec", specName, "issue_code", issues[0].Code)
		return getendpoint.Response{}, fmt.Errorf("getendpoint: reparse stored spec: %s", issues[0].Code)
	}

	method2, path2, ok := openapi.FindOperation(doc, p.method, p.path, p.operationID)
	if !ok {
		eps, err := u.repo.ListSpecEndpoints(ctx, spec.ID)
		if err != nil {
			return getendpoint.Response{}, fmt.Errorf("getendpoint: list endpoints: %w", err)
		}
		sim := openapi.Similar(toCandidates(eps), p.method, p.path, p.operationID, maxSimilar)
		return getendpoint.Response{
			Reason:  getendpoint.ReasonEndpointNotFound,
			Spec:    specName,
			Specs:   []string{},
			Similar: toRefs(sim),
		}, nil
	}

	r, err := openapi.ResolveOperation(doc, method2, path2, p.depth, p.maxBytes)
	if err != nil {
		return getendpoint.Response{}, fmt.Errorf("getendpoint: resolve operation: %w", err)
	}
	res := getendpoint.Response{
		Found:           true,
		Specs:           []string{},
		Similar:         []getendpoint.EndpointRef{},
		Spec:            specName,
		Method:          strings.ToUpper(method2),
		Path:            path2,
		Operation:       r.Operation,
		Servers:         r.Servers,
		SecuritySchemes: r.SecuritySchemes,
		Truncated:       r.Truncated,
		EffectiveDepth:  r.EffectiveDepth,
	}
	if len(res.Servers) == 0 {
		res.Servers = []byte("[]")
	}
	if len(res.SecuritySchemes) == 0 {
		res.SecuritySchemes = []byte("{}")
	}
	if r.Truncated {
		res.Hint = fmt.Sprintf("hasil dipotong; turunkan schema_depth (dipakai %d)", r.EffectiveDepth)
	}
	return res, nil
}

func notFoundSpec(reason string, names []string) getendpoint.Response {
	return getendpoint.Response{
		Reason:  reason,
		Specs:   nonNil(names),
		Similar: []getendpoint.EndpointRef{},
	}
}

func toCandidates(eps []base.Endpoint) []openapi.Candidate {
	out := make([]openapi.Candidate, 0, len(eps))
	for _, e := range eps {
		id := ""
		if e.OperationID != nil {
			id = *e.OperationID
		}
		out = append(out, openapi.Candidate{Method: e.Method, Path: e.Path, OperationID: id, Summary: e.Summary})
	}
	return out
}

func toRefs(cs []openapi.Candidate) []getendpoint.EndpointRef {
	out := make([]getendpoint.EndpointRef, 0, len(cs))
	for _, c := range cs {
		out = append(out, getendpoint.EndpointRef{Method: c.Method, Path: c.Path, OperationID: c.OperationID, Summary: c.Summary})
	}
	return out
}
