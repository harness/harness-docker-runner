module github.com/harness/harness-docker-runner

go 1.17

replace github.com/docker/docker => github.com/docker/engine v17.12.0-ce-rc1.0.20200309214505-aa6a9891b09c+incompatible

require (
	github.com/cenkalti/backoff/v4 v4.2.0
	github.com/docker/distribution v2.8.1+incompatible
	github.com/docker/docker v20.10.21+incompatible
	// this is fake as we are using github.com/docker/engine, this makes the security warning go away
	github.com/docker/go-connections v0.4.0
	github.com/drone/drone-go v1.7.1
	github.com/drone/runner-go v1.12.0
	github.com/go-chi/chi v1.5.4
	github.com/gofrs/uuid v4.3.1+incompatible
	github.com/golang/mock v1.6.0
	github.com/harness/godotenv/v3 v3.0.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/linkedin/goavro/v2 v2.12.0
	github.com/mholt/archiver/v3 v3.5.1
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.9.0
	github.com/stretchr/testify v1.9.0
	golang.org/x/sync v0.1.0
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/yaml.v2 v2.4.0
)

require github.com/dchest/uniuri v0.0.0-20200228104902-7aecb25e1fe5

require (
	github.com/andybalholm/brotli v1.0.4 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dsnet/compress v0.0.2-0.20210315054119-f66993602bf5 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/gops v0.3.25 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/harness/lite-engine v0.4.9-0.20230131094531-9c0bf7135df4 // indirect
	github.com/keybase/go-ps v0.0.0-20190827175125-91aafc93ba19 // indirect
	github.com/klauspost/compress v1.15.14 // indirect
	github.com/klauspost/pgzip v1.2.5 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/nwaples/rardecode v1.1.3 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pierrec/lz4/v4 v4.1.17 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/shirou/gopsutil/v3 v3.22.4 // indirect
	github.com/t-tomalak/logrus-easy-formatter v0.0.0-20190827215021-c074f06c5816 // indirect
	github.com/tklauser/go-sysconf v0.3.10 // indirect
	github.com/tklauser/numcpus v0.4.0 // indirect
	github.com/ulikunitz/xz v0.5.11 // indirect
	github.com/wings-software/dlite v1.0.0-rc.1.0.20230203100932-b8ab0f1060c8 // indirect
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	github.com/xlab/treeprint v1.1.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.2 // indirect
	golang.org/x/crypto v0.0.0-20220315160706-3147a52a75dd // indirect
	golang.org/x/mod v0.7.0 // indirect
	golang.org/x/tools v0.5.0 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/square/go-jose.v2 v2.6.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	rsc.io/goversion v1.2.0 // indirect
)

require (
	github.com/99designs/httpsignatures-go v0.0.0-20170731043157-88528bf4ca7e // indirect
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/Microsoft/go-winio v0.6.0 // indirect
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751 // indirect
	github.com/alecthomas/units v0.0.0-20211218093645-b94a6e3cc137 // indirect
	github.com/bmatcuk/doublestar v1.3.4 // indirect
	github.com/buildkite/yaml v2.1.0+incompatible // indirect
	github.com/containerd/containerd v1.6.15 // indirect
	github.com/coreos/go-semver v0.3.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/drone/envsubst v1.0.3 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/kardianos/service v1.2.2 // indirect
	github.com/mattn/go-zglob v0.0.4
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/natessilva/dag v0.0.0-20180124060714-7194b8dcc5c4 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.0-rc2 // indirect
	golang.org/x/net v0.5.0 // indirect
	golang.org/x/sys v0.4.0 // indirect
	google.golang.org/genproto v0.0.0-20230112194545-e10362b5ecf9 // indirect
	google.golang.org/grpc v1.52.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gotest.tools v2.2.0+incompatible // indirect
)
