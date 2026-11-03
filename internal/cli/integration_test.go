package cli

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/levibostian/jj-spr/internal/runner"
	"github.com/levibostian/mock-a-bin-go"
)

// The scripts below return shapes captured from real jj/gh output during
// real-repo testing, including full `json(self)` commits and realistic branch
// names (`<bookmark>-<change-id>`).
const jjScript = `
args="$*"
case "$args" in
  *"bookmark list"*) printf 'mystack\tlyyuwuqxzwrwsswrpmtomvylxytmulor\nmystack-kpzpsrpmkussxpuuwnktkkpoqwqsonqm\tkpzpsrpmkussxpuuwnktkkpoqwqsonqm\nmystack-smpownmxsvvpymztrypumtosvotxtnto\tsmpownmxsvvpymztrypumtosvotxtnto\nmystack-lyyuwuqxzwrwsswrpmtomvylxytmulor\tlyyuwuqxzwrwsswrpmtomvylxytmulor\n' ;;
  *"bookmark set"*) ;;
  *"git push"*) ;;
  *"git fetch"*) ;;
  *"rebase"*) ;;
  *"conflicts()"*) ;;
  *"~::trunk()"*) echo "lyyuwuqxzwrwsswrpmtomvylxytmulor" ;;
  *"--reversed"*)
    printf '%s' '{"commit_id":"09039837","parents":["00000000"],"change_id":"kpzpsrpmkussxpuuwnktkkpoqwqsonqm","description":"feat: add model\n","author":{"name":"Levi","email":"l@e.com","timestamp":"2026-10-29T09:20:00-05:00"},"committer":{"name":"Levi","email":"l@e.com","timestamp":"2026-10-29T09:20:00-05:00"}}'; printf '\x1f0\x1fk\x1fkpzpsrpm\n'
    printf '%s' '{"commit_id":"9ed1c706","parents":["09039837"],"change_id":"smpownmxsvvpymztrypumtosvotxtnto","description":"feat: add api\n","author":{"name":"Levi","email":"l@e.com","timestamp":"2026-10-29T09:20:01-05:00"},"committer":{"name":"Levi","email":"l@e.com","timestamp":"2026-10-29T09:20:01-05:00"}}'; printf '\x1f0\x1fs\x1fsmpownmx\n'
    printf '%s' '{"commit_id":"7b9292e2","parents":["9ed1c706"],"change_id":"lyyuwuqxzwrwsswrpmtomvylxytmulor","description":"feat: add ui\n","author":{"name":"Levi","email":"l@e.com","timestamp":"2026-10-29T09:20:02-05:00"},"committer":{"name":"Levi","email":"l@e.com","timestamp":"2026-10-29T09:20:02-05:00"}}'; printf '\x1f0\x1fl\x1flyyuwuqx\n' ;;
esac
`

const ghScript = `
args="$*"
case "$args" in
  *"stack --help"*) ;;
  *"repo view"*"defaultBranchRef"*) echo main ;;
  *"repo view"*"owner"*) echo levibostian ;;
  *"pr create"*) count=$(cat /tmp/jjspr-e2e-pr 2>/dev/null || echo 100); count=$((count+1)); echo "$count" > /tmp/jjspr-e2e-pr; echo "https://github.com/levibostian/temp-test-jj-spr/pull/$count" ;;
  *"pr view"*) echo '{"number":101,"title":"feat: add model","state":"OPEN","isDraft":false,"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN","reviewDecision":"","statusCheckRollup":[],"url":"https://github.com/levibostian/temp-test-jj-spr/pull/101","baseRefName":"main"}' ;;
  *"stacks?pull_request"*) echo '[]' ;;
  *"pulls"*) echo '[]' ;;
  *"stack link"*) ;;
esac
`

func mockBinaries(t *testing.T) {
	t.Helper()
	_ = os.Remove("/tmp/jjspr-e2e-pr")
	cleanupJJ, err := mockbin.Mock("jj", "bash", jjScript)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cleanupJJ() })
	cleanupGH, err := mockbin.Mock("gh", "bash", ghScript)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cleanupGH() })
}

// TestEndToEndUpdate runs the real exec runner against mocked jj/gh binaries,
// exercising command building and parsing end to end.
func TestEndToEndUpdate(t *testing.T) {
	mockBinaries(t)
	var out, errb bytes.Buffer
	app := &App{Runner: runner.Exec{}, Stdout: &out, Stderr: &errb}
	if code := app.Run(context.Background(), []string{"update", "--base", "main"}); code != 0 {
		t.Fatalf("code %d stderr=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "created: [101 102 103]") {
		t.Fatalf("out %q", out.String())
	}
}

func TestEndToEndStatus(t *testing.T) {
	mockBinaries(t)
	var out, errb bytes.Buffer
	app := &App{Runner: runner.Exec{}, Stdout: &out, Stderr: &errb}
	if code := app.Run(context.Background(), []string{"status", "--base", "main"}); code != 0 {
		t.Fatalf("code %d stderr=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "mystack") {
		t.Fatalf("out %q", out.String())
	}
}
