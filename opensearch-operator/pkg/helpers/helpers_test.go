package helpers

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	opensearchv1 "github.com/opensearch-project/opensearch-k8s-operator/opensearch-operator/api/opensearch.org/v1"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

var _ = Describe("ClusterURL", func() {
	It("should use operatorClusterURL when provided", func() {
		customHost := "opensearch.example.com"
		cluster := &opensearchv1.OpenSearchCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			Spec: opensearchv1.ClusterSpec{
				General: opensearchv1.GeneralConfig{
					OperatorClusterURL: &customHost,
					HttpPort:           9443,
					ServiceName:        "test",
				},
			},
		}

		result := ClusterURL(cluster)
		Expect(result).To(Equal("http://opensearch.example.com:9443"))
	})

	It("should use default internal DNS when operatorClusterURL is nil", func() {
		cluster := &opensearchv1.OpenSearchCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			Spec: opensearchv1.ClusterSpec{
				General: opensearchv1.GeneralConfig{
					HttpPort:    9200,
					ServiceName: "test",
				},
			},
		}

		result := ClusterURL(cluster)
		Expect(result).To(Equal("http://test.default.svc.cluster.local:9200"))
	})

	It("should use default port 9200 when HttpPort is 0", func() {
		customHost := "opensearch.example.com"
		cluster := &opensearchv1.OpenSearchCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			Spec: opensearchv1.ClusterSpec{
				General: opensearchv1.GeneralConfig{
					OperatorClusterURL: &customHost,
					ServiceName:        "test",
				},
			},
		}

		result := ClusterURL(cluster)
		Expect(result).To(Equal("http://opensearch.example.com:9200"))
	})
})

var _ = Describe("Helper Functions", func() {

	Describe("ResolveUidGid", func() {
		Context("when no security context is specified", func() {
			It("should return default values", func() {
				cluster := &opensearchv1.OpenSearchCluster{
					Spec: opensearchv1.ClusterSpec{
						General: opensearchv1.GeneralConfig{},
					},
				}

				uid, gid := ResolveUidGid(cluster)
				Expect(uid).To(Equal(DefaultUID))
				Expect(gid).To(Equal(DefaultGID))
			})
		})

		Context("when only container security context is specified", func() {
			It("should use container security context values", func() {
				cluster := &opensearchv1.OpenSearchCluster{
					Spec: opensearchv1.ClusterSpec{
						General: opensearchv1.GeneralConfig{
							SecurityContext: &corev1.SecurityContext{
								RunAsUser:  ptr.To(int64(2000)),
								RunAsGroup: ptr.To(int64(2000)),
							},
						},
					},
				}

				uid, gid := ResolveUidGid(cluster)
				Expect(uid).To(Equal(int64(2000)))
				Expect(gid).To(Equal(int64(2000)))
			})
		})

		Context("when only pod security context is specified", func() {
			It("should use pod security context values", func() {
				cluster := &opensearchv1.OpenSearchCluster{
					Spec: opensearchv1.ClusterSpec{
						General: opensearchv1.GeneralConfig{
							PodSecurityContext: &corev1.PodSecurityContext{
								RunAsUser:  ptr.To(int64(1500)),
								RunAsGroup: ptr.To(int64(1500)),
							},
						},
					},
				}

				uid, gid := ResolveUidGid(cluster)
				Expect(uid).To(Equal(int64(1500)))
				Expect(gid).To(Equal(int64(1500)))
			})
		})

		Context("when both security contexts are specified", func() {
			It("should prioritize container security context over pod security context", func() {
				cluster := &opensearchv1.OpenSearchCluster{
					Spec: opensearchv1.ClusterSpec{
						General: opensearchv1.GeneralConfig{
							PodSecurityContext: &corev1.PodSecurityContext{
								RunAsUser:  ptr.To(int64(1500)),
								RunAsGroup: ptr.To(int64(1500)),
							},
							SecurityContext: &corev1.SecurityContext{
								RunAsUser:  ptr.To(int64(3000)),
								RunAsGroup: ptr.To(int64(3000)),
							},
						},
					},
				}

				uid, gid := ResolveUidGid(cluster)
				Expect(uid).To(Equal(int64(3000)))
				Expect(gid).To(Equal(int64(3000)))
			})
		})

		Context("when security contexts have partial values", func() {
			It("should use container UID and pod GID when container GID is missing", func() {
				cluster := &opensearchv1.OpenSearchCluster{
					Spec: opensearchv1.ClusterSpec{
						General: opensearchv1.GeneralConfig{
							PodSecurityContext: &corev1.PodSecurityContext{
								RunAsUser:  ptr.To(int64(1500)),
								RunAsGroup: ptr.To(int64(1800)),
							},
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: ptr.To(int64(2500)),
								// RunAsGroup not specified
							},
						},
					},
				}

				uid, gid := ResolveUidGid(cluster)
				Expect(uid).To(Equal(int64(2500))) // From container context
				Expect(gid).To(Equal(int64(1800))) // From pod context (fallback)
			})

			It("should use defaults when only empty security contexts are provided", func() {
				cluster := &opensearchv1.OpenSearchCluster{
					Spec: opensearchv1.ClusterSpec{
						General: opensearchv1.GeneralConfig{
							PodSecurityContext: &corev1.PodSecurityContext{},
							SecurityContext:    &corev1.SecurityContext{},
						},
					},
				}

				uid, gid := ResolveUidGid(cluster)
				Expect(uid).To(Equal(DefaultUID))
				Expect(gid).To(Equal(DefaultGID))
			})
		})
	})

	Describe("GetChownCommand", func() {
		Context("with valid UID and GID", func() {
			It("should generate correct chown command with default values", func() {
				command := GetChownCommand(1000, 1000, "/usr/share/opensearch/data")
				Expect(command).To(Equal("chown -R 1000:1000 /usr/share/opensearch/data"))
			})
		})
	})

	Describe("MergeConfigs mutation behavior", func() {
		It("should merge the maps such that right is higher priority than left, and not mutate either argument when merging", func() {
			generalConfig := map[string]string{"http.compression": "true"}
			poolConfig := map[string]string{"node.data": "false"}

			// Save a copy of the original
			original := map[string]string{"http.compression": "true"}

			// Merge and check result
			merged := MergeConfigs(generalConfig, poolConfig)
			expected := map[string]string{"http.compression": "true", "node.data": "false"}
			Expect(merged).To(Equal(expected))

			// Check that longLived was not mutated
			Expect(generalConfig).To(Equal(original))

			// Merge again with a new config
			poolConfig2 := map[string]string{"node.master": "false", "http.compression": "false"}
			expected2 := map[string]string{"http.compression": "false", "node.master": "false"}
			merged2 := MergeConfigs(generalConfig, poolConfig2)
			Expect(merged2).To(Equal(expected2))

			// Still not mutated
			Expect(generalConfig).To(Equal(original))
		})
	})

})

var _ = Describe("JVM Heap Size Functions", func() {
	Describe("AppendJvmHeapSizeSettings", func() {
		Context("when JVM string already contains Xmx", func() {
			It("should return the original JVM string unchanged", func() {
				jvm := "-XX:+UseG1GC -Xmx2g -XX:MaxDirectMemorySize=1g"
				heapSizeSettings := "-Xms1g -Xmx2g"

				result := AppendJvmHeapSizeSettings(jvm, heapSizeSettings)

				Expect(result).To(Equal(jvm))
			})
		})

		Context("when JVM string already contains Xms", func() {
			It("should return the original JVM string unchanged", func() {
				jvm := "-XX:+UseG1GC -Xms1g -XX:MaxDirectMemorySize=1g"
				heapSizeSettings := "-Xms1g -Xmx2g"

				result := AppendJvmHeapSizeSettings(jvm, heapSizeSettings)

				Expect(result).To(Equal(jvm))
			})
		})

		Context("when JVM string is empty", func() {
			It("should return only the heap size settings", func() {
				jvm := ""
				heapSizeSettings := "-Xmx1g -Xms1g"

				result := AppendJvmHeapSizeSettings(jvm, heapSizeSettings)

				Expect(result).To(Equal(heapSizeSettings))
			})
		})

		Context("when JVM string does not contain Xmx or Xms", func() {
			It("should append the heap size settings", func() {
				jvm := "-XX:+UseG1GC -XX:MaxDirectMemorySize=1g"
				heapSizeSettings := "-Xmx1g -Xms1g"
				expected := "-XX:+UseG1GC -XX:MaxDirectMemorySize=1g -Xmx1g -Xms1g"

				result := AppendJvmHeapSizeSettings(jvm, heapSizeSettings)

				Expect(result).To(Equal(expected))
			})
		})
	})

	Describe("CalculateJvmHeapSizeSettings", func() {
		Context("when memory request is nil", func() {
			It("should return default 512M for both Xms and Xmx", func() {
				result := CalculateJvmHeapSizeSettings(nil)

				Expect(result).To(Equal("-Xms512M -Xmx512M"))
			})
		})

		Context("when memory request is zero", func() {
			It("should return default 512M for both Xms and Xmx", func() {
				memoryRequest := resource.MustParse("0")

				result := CalculateJvmHeapSizeSettings(&memoryRequest)

				Expect(result).To(Equal("-Xms512M -Xmx512M"))
			})
		})

		Context("when memory request is provided", func() {
			It("should calculate both Xms and Xmx from request", func() {
				memoryRequest := resource.MustParse("2Gi")

				result := CalculateJvmHeapSizeSettings(&memoryRequest)

				Expect(result).To(Equal("-Xms1024M -Xmx1024M"))
			})
		})
	})
})

// Regression coverage for #1371: applyUserHashes must preserve users beyond admin and
// kibanaserver, and any extra fields on admin/kibanaserver themselves.
var _ = Describe("applyUserHashes", func() {
	var (
		adminHashOverride      string
		dashboardsHashOverride string
	)

	BeforeEach(func() {
		// Real bcrypt fixtures so assertions stay meaningful if hash validation is ever
		// added to the override path.
		ah, err := bcrypt.GenerateFromPassword([]byte("admin-pw"), bcrypt.MinCost)
		Expect(err).ToNot(HaveOccurred())
		adminHashOverride = string(ah)

		dh, err := bcrypt.GenerateFromPassword([]byte("dashboards-pw"), bcrypt.MinCost)
		Expect(err).ToNot(HaveOccurred())
		dashboardsHashOverride = string(dh)
	})

	const inputWithCustomUser = `_meta:
  type: "internalusers"
  config_version: 2
admin:
  hash: "placeholder"
  reserved: true
  backend_roles: ["admin"]
  description: "Admin user"
kibanaserver:
  hash: "placeholder"
  reserved: true
  description: "Demo user for the OpenSearch Dashboards server"
dataprepper:
  hash: "$2a$12$existingDataPrepperHashThatMustNotBeTouchedXXXXXXXXXXXXXXX"
  reserved: false
  hidden: false
  backend_roles: ["ingestor"]
  description: "Data Prepper service user"
`

	It("preserves custom users that are neither admin nor kibanaserver", func() {
		out, err := applyUserHashes([]byte(inputWithCustomUser), nil, adminHashOverride, nil, dashboardsHashOverride)
		Expect(err).ToNot(HaveOccurred())

		decoded := decodeYAML(out)
		dp, ok := decoded["dataprepper"].(map[any]any)
		Expect(ok).To(BeTrue(), "dataprepper should still be a mapping")
		Expect(dp["hash"]).To(Equal("$2a$12$existingDataPrepperHashThatMustNotBeTouchedXXXXXXXXXXXXXXX"))
		Expect(dp["description"]).To(Equal("Data Prepper service user"))
		Expect(dp["reserved"]).To(Equal(false))
		Expect(dp["hidden"]).To(Equal(false))
		Expect(dp["backend_roles"]).To(Equal([]any{"ingestor"}))
	})

	It("applies the admin hash override and keeps reserved/admin role invariants", func() {
		out, err := applyUserHashes([]byte(inputWithCustomUser), nil, adminHashOverride, nil, dashboardsHashOverride)
		Expect(err).ToNot(HaveOccurred())

		admin := decodeYAML(out)["admin"].(map[any]any)
		Expect(admin["hash"]).To(Equal(adminHashOverride))
		Expect(admin["reserved"]).To(Equal(true))
		Expect(admin["backend_roles"]).To(ContainElement("admin"))
	})

	It("applies the dashboards hash override, fills the default description, and keeps custom users", func() {
		const stripped = `_meta:
  type: "internalusers"
  config_version: 2
admin:
  hash: "placeholder"
kibanaserver:
  hash: "placeholder"
dataprepper:
  hash: "$2a$12$existingDataPrepperHashThatMustNotBeTouchedXXXXXXXXXXXXXXX"
  backend_roles: ["ingestor"]
`
		out, err := applyUserHashes([]byte(stripped), nil, adminHashOverride, nil, dashboardsHashOverride)
		Expect(err).ToNot(HaveOccurred())

		decoded := decodeYAML(out)
		ks := decoded["kibanaserver"].(map[any]any)
		Expect(ks["hash"]).To(Equal(dashboardsHashOverride))
		Expect(ks["reserved"]).To(Equal(true))
		Expect(ks["description"]).To(Equal("Demo user for the OpenSearch Dashboards server"))
		Expect(decoded).To(HaveKey("dataprepper"))
	})

	It("preserves unrelated fields on the admin entry (e.g. opendistro_security_roles)", func() {
		const richAdmin = `admin:
  hash: "placeholder"
  reserved: true
  backend_roles: ["admin"]
  opendistro_security_roles: ["all_access"]
  attributes:
    department: "platform"
kibanaserver:
  hash: "placeholder"
`
		out, err := applyUserHashes([]byte(richAdmin), nil, adminHashOverride, nil, dashboardsHashOverride)
		Expect(err).ToNot(HaveOccurred())

		admin := decodeYAML(out)["admin"].(map[any]any)
		Expect(admin["hash"]).To(Equal(adminHashOverride))
		Expect(admin).To(HaveKey("opendistro_security_roles"))
		Expect(admin).To(HaveKey("attributes"))
	})

	It("adds the admin backend role when the entry has no backend_roles", func() {
		const noRoles = `admin: {hash: "placeholder"}
kibanaserver: {hash: "placeholder"}
`
		out, err := applyUserHashes([]byte(noRoles), nil, adminHashOverride, nil, dashboardsHashOverride)
		Expect(err).ToNot(HaveOccurred())

		admin := decodeYAML(out)["admin"].(map[any]any)
		Expect(admin["backend_roles"]).To(Equal([]any{"admin"}))
	})

	It("does not duplicate the admin backend role when it is already present", func() {
		const alreadyHasRole = `admin:
  hash: "placeholder"
  backend_roles: ["admin", "ops"]
kibanaserver:
  hash: "placeholder"
`
		out, err := applyUserHashes([]byte(alreadyHasRole), nil, adminHashOverride, nil, dashboardsHashOverride)
		Expect(err).ToNot(HaveOccurred())

		admin := decodeYAML(out)["admin"].(map[any]any)
		Expect(admin["backend_roles"]).To(ConsistOf("admin", "ops"))
	})
})

// decodeYAML unmarshals a yaml.v2 document into a generic map for assertions.
func decodeYAML(b []byte) map[string]any {
	out := map[string]any{}
	ExpectWithOffset(1, yaml.Unmarshal(b, &out)).To(Succeed())
	return out
}
