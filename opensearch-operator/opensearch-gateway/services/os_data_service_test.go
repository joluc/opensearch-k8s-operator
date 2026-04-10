package services

/*
import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"strings"
	"time"
)

var _ = Describe("OpensearchCLuster data service tests", func() {
	//	ctx := context.Background()

	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		timeout  = time.Second * 120
		interval = time.Second * 1
	)

	var (
		ClusterClient *OsClusterClient = nil
	)

	/// ------- Creation Check phase -------

	BeforeEach(func() {
		By("Creating open search client ")
		Eventually(func() bool {
			clusterClient, err := NewOsClusterClient(TestClusterUrl, TestClusterUserName, TestClusterPassword)
			if err != nil {
				return false
			}
			ClusterClient = clusterClient
			return true
		}, timeout, interval).Should(BeTrue())
	})
	Context("Data Service Tests logic", func() {
		It("Test Has No Indices With No Replica", func() {
			mapping := strings.NewReader(`{
											 "settings": {
											   "index": {
													"number_of_shards": 1,
													"number_of_replicas": 1
													}
												  }
											 }`)
			indexName := "indices-no-rep-test"
			_, err := DeleteIndex(ClusterClient, indexName)
			Expect(err).Should(BeNil())
			success, err := CreateIndex(ClusterClient, indexName, mapping)
			Expect(err).Should(BeNil())
			Expect(success == 200 || success == 201).Should(BeTrue())
			hasNoReplicas, err := HasIndicesWithNoReplica(ClusterClient)
			Expect(err).Should(BeNil())
			Expect(hasNoReplicas).ShouldNot(BeTrue())
			_, err = DeleteIndex(ClusterClient, indexName)
			Expect(err).Should(BeNil())
		})
		It("Test Has Indices With No Replica", func() {
			mapping := strings.NewReader(`{
											 "settings": {
											   "index": {
													"number_of_shards": 1,
													"number_of_replicas": 0
													}
												  }
											 }`)
			indexName := "indices-with-rep-test"
			_, err := DeleteIndex(ClusterClient, indexName)
			Expect(err).Should(BeNil())
			hasNoReplicas := false
			success, err := CreateIndex(ClusterClient, indexName, mapping)
			Expect(err).Should(BeNil())
			Expect(success == 200 || success == 201).Should(BeTrue())
			hasNoReplicas, err = HasIndicesWithNoReplica(ClusterClient)
			Expect(err).Should(BeNil())
			Expect(hasNoReplicas).Should(BeTrue())
			_, err = DeleteIndex(ClusterClient, indexName)
			Expect(err).Should(BeNil())
		})
		It("Test Node Exclude", func() {
			nodeExcluded, err := AppendExcludeNodeHost(ClusterClient, "not-exists-node")
			Expect(err).Should(BeNil())
			Expect(nodeExcluded).Should(BeTrue())
			nodeExcluded, err = RemoveExcludeNodeHost(ClusterClient, "not-exists-node")
			Expect(err).Should(BeNil())
			Expect(nodeExcluded).Should(BeTrue())
		})
	})
})*/

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/opensearch-project/opensearch-k8s-operator/opensearch-operator/opensearch-gateway/responses"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// TestExtractNodeName verifies that the source node name is correctly extracted from
// the _cat/shards API node field, including during shard relocation when the format
// is "sourceNode -> ip id targetNode".
func TestExtractNodeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "plain node name",
			input:    "opensearch-data-1",
			expected: "opensearch-data-1",
		},
		{
			name:     "shard relocation format - extracts source node",
			input:    "opensearch-data-1 -> 172.31.233.51 4kGSHQhmRQ-83pvvBbTYow opensearch-data-8",
			expected: "opensearch-data-1",
		},
		{
			name:     "leading and trailing spaces",
			input:    "  opensearch-data-2  ",
			expected: "opensearch-data-2",
		},
		{
			name:     "relocation format with leading space",
			input:    "  opensearch-data-0 -> 10.0.0.1 abc123 opensearch-data-5",
			expected: "opensearch-data-0",
		},
		{
			name:     "single token",
			input:    "node-a",
			expected: "node-a",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractNodeName(tt.input)
			if got != tt.expected {
				t.Errorf("extractNodeName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// TestHasShardsOnNodeFromResponse verifies that shard-to-node matching correctly uses
// the source node name from the _cat/shards API, including during shard relocation when
// the node field format is "sourceNode -> ip id targetNode" (fix for issue #1133).
func TestHasShardsOnNodeFromResponse(t *testing.T) {
	tests := []struct {
		name     string
		shards   []responses.CatShardsResponse
		nodeName string
		want     bool
	}{
		{
			name:     "no shards - returns false",
			shards:   nil,
			nodeName: "opensearch-data-1",
			want:     false,
		},
		{
			name:     "empty shards - returns false",
			shards:   []responses.CatShardsResponse{},
			nodeName: "opensearch-data-1",
			want:     false,
		},
		{
			name: "plain node name match",
			shards: []responses.CatShardsResponse{
				{Index: "idx", Shard: "0", PrimaryOrReplica: "p", State: "STARTED", NodeName: "opensearch-data-1"},
			},
			nodeName: "opensearch-data-1",
			want:     true,
		},
		{
			name: "shard relocation format - match source node",
			shards: []responses.CatShardsResponse{
				{Index: "idx", Shard: "0", PrimaryOrReplica: "p", State: "STARTED", NodeName: "opensearch-data-1 -> 172.31.233.51 4kGSHQhmRQ-83pvvBbTYow opensearch-data-8"},
			},
			nodeName: "opensearch-data-1",
			want:     true,
		},
		{
			name: "shard relocation format - target node should not match when querying by name",
			shards: []responses.CatShardsResponse{
				{Index: "idx", Shard: "0", PrimaryOrReplica: "p", State: "STARTED", NodeName: "opensearch-data-1 -> 172.31.233.51 4kGSHQhmRQ opensearch-data-8"},
			},
			nodeName: "opensearch-data-8",
			want:     false,
		},
		{
			name: "multiple shards - one matches source node",
			shards: []responses.CatShardsResponse{
				{Index: "a", Shard: "0", PrimaryOrReplica: "p", State: "STARTED", NodeName: "other-node"},
				{Index: "b", Shard: "0", PrimaryOrReplica: "p", State: "STARTED", NodeName: "opensearch-data-2 -> 10.0.0.1 xyz opensearch-data-9"},
			},
			nodeName: "opensearch-data-2",
			want:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasShardsOnNodeFromResponse(tt.shards, tt.nodeName)
			if got != tt.want {
				t.Errorf("hasShardsOnNodeFromResponse() = %v, want %v", got, tt.want)
			}
		})
	}
}

// newMockOsClient creates an OsClusterClient backed by an httptest server.
// The handler receives all HTTP requests so tests can return canned responses.
func newMockOsClient(t *testing.T, handler http.HandlerFunc) (*OsClusterClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	client, err := NewOsClusterClient(srv.URL, "", "")
	if err != nil {
		t.Fatalf("creating mock client: %v", err)
	}
	return client, srv
}

func TestCheckClusterStatusForRestart(t *testing.T) {
	tests := []struct {
		name      string
		health    responses.ClusterHealthResponse
		drain     bool
		wantReady bool
		wantMsg   string
	}{
		{
			name:      "green cluster, drain off",
			health:    responses.ClusterHealthResponse{Status: "green"},
			drain:     false,
			wantReady: true,
		},
		{
			name:      "green cluster, drain on",
			health:    responses.ClusterHealthResponse{Status: "green"},
			drain:     true,
			wantReady: true,
		},
		{
			name:      "yellow cluster, drain off - allows restart",
			health:    responses.ClusterHealthResponse{Status: "yellow"},
			drain:     false,
			wantReady: true,
		},
		{
			name: "yellow cluster, drain on - blocks restart",
			health: responses.ClusterHealthResponse{
				Status:           "yellow",
				RelocatingShards: 1, // prevents CheckClusterRestartOnYellow from short-circuiting
			},
			drain:     true,
			wantReady: false,
			wantMsg:   "cluster is not green and drain nodes is enabled",
		},
		{
			name:      "red cluster, drain off - blocks restart",
			health:    responses.ClusterHealthResponse{Status: "red"},
			drain:     false,
			wantReady: false,
			wantMsg:   "cluster is red",
		},
		{
			name:      "red cluster, drain on - blocks restart",
			health:    responses.ClusterHealthResponse{Status: "red"},
			drain:     true,
			wantReady: false,
			wantMsg:   "cluster is not green and drain nodes is enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, srv := newMockOsClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/_cluster/health":
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(tt.health)
				default:
					// Ping and info endpoints for client creation
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprintf(w, `{"name":"mock","cluster_name":"mock","version":{"distribution":"opensearch","number":"2.11.0"}}`)
				}
			})
			defer srv.Close()

			ready, msg, err := CheckClusterStatusForRestart(client, tt.drain)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ready != tt.wantReady {
				t.Errorf("ready = %v, want %v", ready, tt.wantReady)
			}
			if msg != tt.wantMsg {
				t.Errorf("msg = %q, want %q", msg, tt.wantMsg)
			}
		})
	}
}

func TestPreparePodForDelete_NonDrain(t *testing.T) {
	lg := log.Log.WithName("test")
	// Non-drain path should return true immediately without calling the service
	ready, err := PreparePodForDelete(nil, lg, "opensearch-nodes-0", false, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ready {
		t.Error("expected ready=true for non-drain path")
	}
}
