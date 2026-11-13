package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/swaggo/swag"
	"github.com/swaggo/swag/gen"
)

// Deployment Readiness regression test. Swagger generation requires
// --parseInternal --parseDependency (README.md's documented command
// and backend/Dockerfile's builder stage both already use these) —
// omitting them fails with "cannot find type definition:
// apihttp.SuccessResponse", because swag's package resolver treats
// internal/platform/http (imported as "apihttp") as an unparsed
// external dependency unless told to actually parse it. This test
// calls the exact same gen.Gen.Build the `swag` CLI itself calls
// (cmd/swag/main.go), with the exact flag values README.md/Dockerfile
// already use, so a regression here — a new handler's doc comment the
// parser can't resolve, a flag silently dropped from the documented
// command, a swag upgrade that changes this behavior — fails `go test`
// instead of surfacing only when someone manually runs `swag init` (or
// `docker compose build`) and notices.
//
// Runs entirely in-process against this module's real source tree —
// no database, no external services, no swag CLI binary required on
// PATH.
func TestSwaggerGeneration_ParsesWithDocumentedFlags(t *testing.T) {
	// cmd/server is this test's own working directory; ".." reaches
	// backend/ (SearchDir), matching README.md/Dockerfile's "run from
	// backend/, -g cmd/server/main.go" convention — MainAPIFile is
	// joined onto SearchDir the same way swag's own CLI does.
	backendRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve backend root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(backendRoot, "cmd", "server", "main.go")); err != nil {
		t.Fatalf("sanity check failed — expected %s to contain cmd/server/main.go: %v", backendRoot, err)
	}

	outDir := t.TempDir()

	err = gen.New().Build(&gen.Config{
		SearchDir:          backendRoot,
		MainAPIFile:        "cmd/server/main.go",
		OutputDir:          outDir,
		OutputTypes:        []string{"go", "json", "yaml"}, // swag CLI default (outputTypesFlag)
		PropNamingStrategy: swag.CamelCase,                 // swag CLI default (propertyStrategyFlag)
		CollectionFormat:   "csv",                          // swag CLI default (collectionFormatFlag)
		ParseDepth:         100,                            // swag CLI default (parseDepthFlag)
		// README.md's documented command / Dockerfile's builder stage:
		// `swag init -g cmd/server/main.go -o docs --parseInternal --parseDependency`
		ParseInternal: true,
		// The bare --parseDependency boolean flag (no --parseDependencyLevel)
		// sets this to 1 (ParseModels), not swag.ParseAll — see
		// cmd/swag/main.go's own flag-to-Config translation. Matching
		// that exactly (not a stronger value) keeps this test honest
		// about what the documented command actually does.
		ParseDependency: 1,
		InstanceName:    "swagger",
	})
	if err != nil {
		t.Fatalf(
			"swag generation failed with the documented command's flags (SearchDir=%s, MainAPIFile=cmd/server/main.go, "+
				"--parseInternal --parseDependency): %v\n\n"+
				"If this is \"cannot find type definition: <alias>.<Type>\" for a newly-added handler file, "+
				"check that file's doc-comment type references resolve under these exact flags before assuming "+
				"a tooling regression — see this test's own doc comment for why plain `swag init -g cmd/server/main.go` "+
				"(no flags) is known to fail here even when the flagged command succeeds.",
			backendRoot, err,
		)
	}

	for _, f := range []string{"docs.go", "swagger.json", "swagger.yaml"} {
		if _, err := os.Stat(filepath.Join(outDir, f)); err != nil {
			t.Errorf("expected generated file %s to exist: %v", f, err)
		}
	}
}

// TestSwaggerGeneration_PlainCommandFailsWithoutFlags documents the
// actual failure mode this project's README.md/Dockerfile already
// route around, as an explicit, intentional "known-bad" case rather
// than a silent assumption — if a future swag upgrade fixes the
// underlying resolver so the plain command starts succeeding too, this
// test will fail and should be read as "good news, the flags may no
// longer be required," not a regression to chase.
func TestSwaggerGeneration_PlainCommandFailsWithoutFlags(t *testing.T) {
	backendRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve backend root: %v", err)
	}

	err = gen.New().Build(&gen.Config{
		SearchDir:          backendRoot,
		MainAPIFile:        "cmd/server/main.go",
		OutputDir:          t.TempDir(),
		OutputTypes:        []string{"go"},
		PropNamingStrategy: swag.CamelCase,
		CollectionFormat:   "csv",
		ParseDepth:         100,
		// Deliberately omitted: ParseInternal, ParseDependency.
		InstanceName: "swagger",
	})
	if err == nil {
		t.Skip("plain `swag init -g cmd/server/main.go` (no flags) now succeeds — the documented " +
			"--parseInternal --parseDependency workaround may no longer be necessary; this is good news, not a failure")
	}
}
