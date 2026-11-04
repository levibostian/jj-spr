package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levibostian/jj-spr/internal/runner"
	"github.com/levibostian/mock-a-bin-go"
)

// TestEndToEndStatusMidStack reproduces the reported "multiple stack bookmarks
// found" state: @ is parked mid-stack on ymosozmx, a per-commit tool bookmark
// sits next to it, and unrelated bookmarks exist elsewhere. Detection must pick
// the fixtest stack, not error.
//
// The fixtures are trimmed, verbatim captures from a real run against
// levibostian/temp-test-jj-spr. The fixtest stack is:
// main -> oqxqzqry (fix model, #41) -> ymosozmx (fix ui, #43) -> ...
func TestEndToEndStatusMidStack(t *testing.T) {
	// Whole real trunk()..orqrlonr log: the entire four-PR fixtest stack plus
	// the empty commit between fix api and fix docs.
	logOut := `{"commit_id":"e6d9f225e09f00fca5fbc8bfdafe0ca138600c91","parents":["58ced7dae83bae7c66e04e9c47da191617e85dc6"],"change_id":"oqxqzqrylvypnqowktnxnqsuwnsqqsuk","description":"feat: fix model\n","author":{"name":"Levi Bostian","email":"levi@levi.earth","timestamp":"2026-10-29T13:01:46-05:00"},"committer":{"name":"Levi Bostian","email":"levi@levi.earth","timestamp":"2026-10-29T13:01:47-05:00"}}` + "\x1f0\x1foq\x1foqxqzqry\n" +
		`{"commit_id":"47e563dc9eb13a04f7dcd1abe9ce8c67096c01f4","parents":["e6d9f225e09f00fca5fbc8bfdafe0ca138600c91"],"change_id":"ymosozmxswzlttprpomytvqovlvkklpl","description":"feat: fix ui\n","author":{"name":"Levi Bostian","email":"levi@levi.earth","timestamp":"2026-10-29T13:01:47-05:00"},"committer":{"name":"Levi Bostian","email":"levi@levi.earth","timestamp":"2026-10-29T13:02:27-05:00"}}` + "\x1f0\x1fym\x1fymosozmx\n" +
		`{"commit_id":"ccf55f8eb6388d8054816277ac3e79533709aa27","parents":["47e563dc9eb13a04f7dcd1abe9ce8c67096c01f4"],"change_id":"lrzmrvsptrvmqotxorzxvkvprqmoqsup","description":"feat: fix api\n","author":{"name":"Levi Bostian","email":"levi@levi.earth","timestamp":"2026-10-29T13:01:47-05:00"},"committer":{"name":"Levi Bostian","email":"levi@levi.earth","timestamp":"2026-10-29T13:02:27-05:00"}}` + "\x1f0\x1fl\x1flrzmrvsp\n" +
		`{"commit_id":"a925eae7d1cb0341ec0918675a0c873b19a3cd9f","parents":["ccf55f8eb6388d8054816277ac3e79533709aa27"],"change_id":"ykvwmvotppnzqkvkunxlspmutztwvuuy","description":"","author":{"name":"Levi Bostian","email":"levi@levi.earth","timestamp":"2026-10-29T13:02:27-05:00"},"committer":{"name":"Levi Bostian","email":"levi@levi.earth","timestamp":"2026-10-29T13:02:27-05:00"}}` + "\x1f1\x1fyk\x1fykvwmvot\n" +
		`{"commit_id":"2a6f852b3cfc084a1ad6489a17fab85b642f377e","parents":["a925eae7d1cb0341ec0918675a0c873b19a3cd9f"],"change_id":"orqrlonrvsrqqvxyzlkxlyyvzntyopww","description":"feat: fix docs\n","author":{"name":"Levi Bostian","email":"levi@levi.earth","timestamp":"2026-10-29T13:25:21-05:00"},"committer":{"name":"Levi Bostian","email":"levi@levi.earth","timestamp":"2026-10-29T13:25:21-05:00"}}` + "\x1f0\x1for\x1forqrlonr\n"

	pullsModel := `[{"number":41,"state":"open","merged_at":null,"draft":false,"title":"feat: fix model","body":null,"html_url":"https://github.com/levibostian/temp-test-jj-spr/pull/41","head":{"ref":"fixtest-oqxqzqrylvypnqowktnxnqsuwnsqqsuk"},"base":{"ref":"main"}}]`
	pullsUI := `[{"number":43,"state":"open","merged_at":null,"draft":false,"title":"feat: fix ui","body":null,"html_url":"https://github.com/levibostian/temp-test-jj-spr/pull/43","head":{"ref":"fixtest-ymosozmxswzlttprpomytvqovlvkklpl"},"base":{"ref":"fixtest-oqxqzqrylvypnqowktnxnqsuwnsqqsuk"}}]`
	pullsAPI := `[{"number":42,"state":"open","merged_at":null,"draft":false,"title":"feat: fix api","body":null,"html_url":"https://github.com/levibostian/temp-test-jj-spr/pull/42","head":{"ref":"fixtest-lrzmrvsptrvmqotxorzxvkvprqmoqsup"},"base":{"ref":"fixtest-ymosozmxswzlttprpomytvqovlvkklpl"}}]`
	pullsDocs := `[{"number":46,"state":"open","merged_at":null,"draft":false,"title":"feat: fix docs","body":null,"html_url":"https://github.com/levibostian/temp-test-jj-spr/pull/46","head":{"ref":"fixtest-orqrlonrvsrqqvxyzlkxlyyvzntyopww"},"base":{"ref":"fixtest-lrzmrvsptrvmqotxorzxvkvprqmoqsup"}}]`
	viewModel := `{"baseRefName":"main","isDraft":false,"mergeStateStatus":"CLEAN","mergeable":"MERGEABLE","number":41,"reviewDecision":"","state":"OPEN","statusCheckRollup":[],"title":"feat: fix model","url":"https://github.com/levibostian/temp-test-jj-spr/pull/41"}`
	viewUI := `{"baseRefName":"fixtest-oqxqzqrylvypnqowktnxnqsuwnsqqsuk","isDraft":false,"mergeStateStatus":"CLEAN","mergeable":"MERGEABLE","number":43,"reviewDecision":"","state":"OPEN","statusCheckRollup":[],"title":"feat: fix ui","url":"https://github.com/levibostian/temp-test-jj-spr/pull/43"}`
	viewAPI := `{"baseRefName":"fixtest-ymosozmxswzlttprpomytvqovlvkklpl","isDraft":false,"mergeStateStatus":"CLEAN","mergeable":"MERGEABLE","number":42,"reviewDecision":"","state":"OPEN","statusCheckRollup":[],"title":"feat: fix api","url":"https://github.com/levibostian/temp-test-jj-spr/pull/42"}`
	viewDocs := `{"baseRefName":"fixtest-lrzmrvsptrvmqotxorzxvkvprqmoqsup","isDraft":false,"mergeStateStatus":"CLEAN","mergeable":"MERGEABLE","number":46,"reviewDecision":"","state":"OPEN","statusCheckRollup":[],"title":"feat: fix docs","url":"https://github.com/levibostian/temp-test-jj-spr/pull/46"}`

	logFile := filepath.Join(t.TempDir(), "log.txt")
	if err := os.WriteFile(logFile, []byte(logOut), 0o644); err != nil {
		t.Fatal(err)
	}
	jjScript := fmt.Sprintf(`
args="$*"
case "$args" in
  *"--reversed"*) cat %q ;;
  *"::@ | @::"*) printf 'qyqkxmwnppmyoqvsvrkzusvwxuxxxpyz\x1fsibstack\norqrlonrvsrqqvxyzlkxlyyvzntyopww\x1ffixtest fixtest-orqrlonrvsrqqvxyzlkxlyyvzntyopww\n' ;;
  *"::@"*) printf 'ymosozmxswzlttprpomytvqovlvkklpl\x1ffixtest-ymosozmxswzlttprpomytvqovlvkklpl\n' ;;
esac
`, logFile)
	ghScript := `
args="$*"
case "$args" in
  *"repo view"*"owner"*) echo levibostian ;;
  *"pr view"*"fixtest-oqxqzqrylvypnqowktnxnqsuwnsqqsuk"*) echo '` + viewModel + `' ;;
  *"pr view"*"fixtest-ymosozmxswzlttprpomytvqovlvkklpl"*) echo '` + viewUI + `' ;;
  *"pr view"*"fixtest-lrzmrvsptrvmqotxorzxvkvprqmoqsup"*) echo '` + viewAPI + `' ;;
  *"pr view"*) echo '` + viewDocs + `' ;;
  *"fixtest-oqxqzqrylvypnqowktnxnqsuwnsqqsuk"*) echo '` + pullsModel + `' ;;
  *"fixtest-ymosozmxswzlttprpomytvqovlvkklpl"*) echo '` + pullsUI + `' ;;
  *"fixtest-lrzmrvsptrvmqotxorzxvkvprqmoqsup"*) echo '` + pullsAPI + `' ;;
  *"fixtest-orqrlonrvsrqqvxyzlkxlyyvzntyopww"*) echo '` + pullsDocs + `' ;;
esac
`
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

	var out, errb bytes.Buffer
	app := &App{Runner: runner.Exec{}, Stdout: &out, Stderr: &errb}
	if code := app.Run(context.Background(), []string{"status", "--base", "main"}); code != 0 {
		t.Fatalf("code %d stderr=%s", code, errb.String())
	}
	got := out.String()
	if strings.Contains(got, "multiple stack bookmarks") {
		t.Fatalf("detection failed: %q", got)
	}
	for _, want := range []string{"fixtest", "#41", "#42", "#43", "#46"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in out %q", want, got)
		}
	}
}

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
  *"~::"*) printf 'lyyuwuqxzwrwsswrpmtomvylxytmulor\x1fmystack\n' ;;
  *remote_bookmarks*) echo main ;;
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
