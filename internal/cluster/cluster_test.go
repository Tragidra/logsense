package cluster

import (
	"bufio"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tragidra/loglens/internal/normalize"
	"github.com/Tragidra/loglens/model"
)

func TestTokenize_Basic(t *testing.T) {
	got := Tokenize("user alice failed login from 192.168.1.1")
	assert.Equal(t, []string{"user", "alice", "failed", "login", "from", "192.168.1.1"}, got)
}

func TestTokenize_StripsPunctuation(t *testing.T) {
	got := Tokenize("error: connection refused, retrying.")
	assert.Equal(t, []string{"error", "connection", "refused", "retrying"}, got)
}

func TestTokenize_QuotedSpan(t *testing.T) {
	got := Tokenize(`GET "/api/health check" HTTP/1.1`)
	assert.Equal(t, []string{"GET", `"/api/health check"`, "HTTP/1.1"}, got)
}

func TestTokenize_Empty(t *testing.T) {
	assert.Empty(t, Tokenize(""))
	assert.Empty(t, Tokenize("   "))
}

func TestHasDigit(t *testing.T) {
	assert.True(t, hasDigit("user123"))
	assert.True(t, hasDigit("42"))
	assert.False(t, hasDigit("alice"))
	assert.False(t, hasDigit(""))
}

func TestLooksParameterLike(t *testing.T) {
	cases := []struct {
		tok  string
		want bool
	}{
		{"42", true},   // integer
		{"3.14", true}, // float
		{"0xff", true}, // hex number
		{"550e8400-e29b-41d4-a716-446655440000", true}, // UUID
		{"192.168.1.1", true},                          // IP
		{"10.0.0.1:8080", true},                        // IP:port
		{"2026-04-19T12:00:00", true},                  // ISO timestamp
		{"/var/log/app.log", true},                     // file path
		{"dGhpcyBpcyBhIGJhc2U2NCBibG9iISE=", true},     // base64 (29 chars)
		{"alice", false},                               // plain word
		{"failed", false},                              // plain word
		{"login", false},                               // plain word
		{"db01", false},                                // alphanum but no full match
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, looksParameterLike(tc.tok), "tok=%q", tc.tok)
	}
}

func TestSimilarity_Exact(t *testing.T) {
	tmpl := []string{"user", "alice", "logged", "in"}
	assert.Equal(t, 1.0, similarity(tmpl, tmpl))
}

func TestSimilarity_Wildcard(t *testing.T) {
	tmpl := []string{"user", "<*>", "logged", "in"}
	tokens := []string{"user", "bob", "logged", "in"}
	assert.Equal(t, 1.0, similarity(tmpl, tokens))
}

func TestSimilarity_Mismatch(t *testing.T) {
	tmpl := []string{"user", "alice", "logged", "in"}
	tokens := []string{"user", "bob", "logged", "out"}
	assert.InDelta(t, 0.5, similarity(tmpl, tokens), 0.001)
}

func TestSimilarity_DifferentLength(t *testing.T) {
	assert.Equal(t, 0.0, similarity([]string{"a", "b"}, []string{"a"}))
}

func TestMergeTemplate(t *testing.T) {
	g := &LogGroup{TokensTmpl: []string{"user", "alice", "from", "1.2.3.4"}}
	mergeTemplate(g, []string{"user", "bob", "from", "5.6.7.8"})
	assert.Equal(t, []string{"user", "<*>", "from", "<*>"}, g.TokensTmpl)
}

func TestReservoir_FillsUpToCap(t *testing.T) {
	r := NewReservoir(3)
	r.Add("a")
	r.Add("b")
	r.Add("c")
	assert.Len(t, r.Items(), 3)
}

func TestReservoir_NeverExceedsCap(t *testing.T) {
	r := NewReservoir(5)
	for i := 0; i < 100; i++ {
		r.Add("item")
	}
	assert.LessOrEqual(t, len(r.Items()), 5)
}

func newTree() *Tree { return NewTree(3, 100, 10, 0.4) }

func insert(t *Tree, msg string) *LogGroup {
	return t.Insert(model.LogEvent{Message: msg, Timestamp: time.Now()})
}

func TestTree_Insert_SameTemplateMerges(t *testing.T) {
	tr := newTree()
	insert(tr, "database connected to host db01")
	insert(tr, "database connected to host db02")
	insert(tr, "database connected to host db03")

	gs := tr.AllGroups()
	require.Len(t, gs, 1)
	assert.Equal(t, int64(3), gs[0].Count)
	assert.Equal(t, []string{"database", "connected", "to", "host", "<*>"}, gs[0].TokensTmpl)
}

func TestTree_Insert_DifferentLengthsApart(t *testing.T) {
	tr := newTree()
	insert(tr, "a b c")   // 3 tokens
	insert(tr, "a b c d") // 4 tokens — same bucket 4 for different length
	assert.Len(t, tr.AllGroups(), 2)
}

func TestTree_Insert_EmptyMessage(t *testing.T) {
	tr := newTree()
	g := insert(tr, "")
	require.NotNil(t, g)
	assert.Equal(t, []string{"<empty>"}, g.TokensTmpl)
}

func TestTree_MultipleDistinctTemplates(t *testing.T) {
	tr := newTree()
	lines := []string{
		"login success alice from 192.168.1.1",
		"login success bob from 192.168.1.2",
		"login success charlie from 192.168.1.3",
		"login failed alice from 10.0.0.1",
		"login failed bob from 10.0.0.2",
		"login failed dave from 172.16.0.1",
		"database connected to host db01",
		"database connected to host db02",
		"database connected to host db03",
		"request processed in 42ms status 200",
		"request processed in 58ms status 200",
		"request processed in 91ms status 404",
		"cache miss for session:abc123",
		"cache miss for session:def456",
		"cache miss for user:789",
	}
	for _, l := range lines {
		insert(tr, l)
	}
	assert.Equal(t, 5, len(tr.AllGroups()))
}

func TestTree_SaveLoad(t *testing.T) {
	path := t.TempDir() + "/cluster.tree"
	tr := newTree()
	insert(tr, "database connected to host db01")
	insert(tr, "database connected to host db02")

	require.NoError(t, tr.Save(path))

	tr2 := newTree()
	require.NoError(t, tr2.Load(path))

	gs := tr2.AllGroups()
	require.Len(t, gs, 1)
	assert.Equal(t, int64(2), gs[0].Count)
	assert.Equal(t, []string{"database", "connected", "to", "host", "<*>"}, gs[0].TokensTmpl)
}

func TestTree_Prune_RemovesOldGroups(t *testing.T) {
	tr := newTree()
	old := tr.Insert(model.LogEvent{
		Message:   "old event happened",
		Timestamp: time.Now().Add(-2 * time.Hour),
	})
	old.LastSeen = time.Now().Add(-2 * time.Hour)

	insert(tr, "recent event happened now")

	tr.Prune(time.Hour)
	for _, g := range tr.AllGroups() {
		assert.NotEqual(t, "old event happened", g.TokensTmpl[0])
	}
}

func TestGoldenNginxLog(t *testing.T) {
	f, err := os.Open("testdata/drain/nginx.log")
	require.NoError(t, err)
	defer f.Close()

	n := normalize.New()
	tr := newTree()
	receivedAt := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		e := n.Normalize(model.RawLog{Raw: line, Source: "nginx", ReceivedAt: receivedAt})
		tr.Insert(e)
	}
	require.NoError(t, sc.Err())

	// The 15 nginx lines normalise to messages with 3 distinct HTTP methods (GET, POST, DELETE)
	gs := tr.AllGroups()
	assert.Equal(t, 3, len(gs), "expected 3 clusters: GET <*>, POST <*>, DELETE <*>")
}

func BenchmarkInsert(b *testing.B) {
	tr := newTree()
	events := []model.LogEvent{
		{Message: "user alice logged in from 192.168.1.1", Timestamp: time.Now()},
		{Message: "database connected to host db01 port 5432", Timestamp: time.Now()},
		{Message: "request processed in 42ms status 200", Timestamp: time.Now()},
		{Message: "cache miss for key session:abc123", Timestamp: time.Now()},
		{Message: "error connection refused to postgres:5432", Timestamp: time.Now()},
		{Message: "login failed for user bob attempt 3", Timestamp: time.Now()},
		{Message: "queue depth 1024 messages pending", Timestamp: time.Now()},
		{Message: "health check ok latency 5ms", Timestamp: time.Now()},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr.Insert(events[i%len(events)])
	}
}
