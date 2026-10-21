// Package management merakit module project/management: route HTTP dan handler bus.
package management

import (
	"grimoire/internal/domain/project/internal/management/addmember"
	"grimoire/internal/domain/project/internal/management/changememberrole"
	"grimoire/internal/domain/project/internal/management/checkaccess"
	"grimoire/internal/domain/project/internal/management/createproject"
	"grimoire/internal/domain/project/internal/management/deleteproject"
	"grimoire/internal/domain/project/internal/management/getproject"
	"grimoire/internal/domain/project/internal/management/getprojectbus"
	"grimoire/internal/domain/project/internal/management/listmembers"
	"grimoire/internal/domain/project/internal/management/listprojects"
	"grimoire/internal/domain/project/internal/management/removemember"
	"grimoire/internal/domain/project/internal/management/searchmembercandidates"
	"grimoire/internal/domain/project/internal/management/updateproject"
	"grimoire/internal/infra/app"
)

// Register merakit module: handler bus dan route REST (pola penuh di d.API).
func Register(d app.Deps) {
	// Handler bus.
	checkaccess.Init(d)
	getprojectbus.Init(d)

	// Route REST /api/projects...
	listprojects.Init(d)
	createproject.Init(d)
	getproject.Init(d)
	updateproject.Init(d)
	deleteproject.Init(d)
	listmembers.Init(d)
	searchmembercandidates.Init(d)
	addmember.Init(d)
	removemember.Init(d)
	changememberrole.Init(d)
}
