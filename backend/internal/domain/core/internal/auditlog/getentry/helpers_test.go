package getentry

import "grimoire/internal/infra/app"

// testDeps cukup untuk Init: DB nil aman karena repository baru memakainya saat request.
func testDeps() app.Deps { return app.Deps{} }
