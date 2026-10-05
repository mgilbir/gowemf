package corpus

// Oracle artifacts are execution-only dependencies, not Go/library dependencies.
// JARs retain their upstream license/NOTICE resources and remain unshipped.
type Artifact struct {
	Path, SHA256 string
	Bytes        int64
}

const MavenURL = "https://repo.maven.apache.org/maven2/"

var OracleArtifacts = []Artifact{
	{"org/apache/poi/poi/5.4.1/poi-5.4.1.jar", "da5abf42da4604c5a7bca38956af6e9d6f196d9b6d4cb7eabee4f480b580d505", 2996461},
	{"org/apache/poi/poi-scratchpad/5.4.1/poi-scratchpad-5.4.1.jar", "6497ba15c1cba7062aa71661a8d776d321b1f998bb2bfa19b57d7e35606381f1", 1909132},
	{"org/apache/logging/log4j/log4j-api/2.24.3/log4j-api-2.24.3.jar", "5b4a0a0cd0e751ded431c162442bdbdd53328d1f8bb2bae5fc1bbeee0f66d80f", 348513},
	{"org/apache/commons/commons-math3/3.6.1/commons-math3-3.6.1.jar", "1e56d7b058d28b65abd256b8458e3885b674c1d588fa43cd7d1cbb9c7ef2b308", 2213560},
	{"commons-codec/commons-codec/1.18.0/commons-codec-1.18.0.jar", "ba005f304cef92a3dede24a38ad5ac9b8afccf0d8f75839d6c1338634cf7f6e4", 373045},
	{"commons-io/commons-io/2.18.0/commons-io-2.18.0.jar", "f3ca0f8d63c40e23a56d54101c60d5edee136b42d84bfb85bc7963093109cf8b", 538910},
	{"org/apache/commons/commons-collections4/4.4/commons-collections4-4.4.jar", "1df8b9430b5c8ed143d7815e403e33ef5371b2400aadbe9bda0883762e0846d1", 751914},
	{"com/zaxxer/SparseBitSet/1.3/SparseBitSet-1.3.jar", "f76b85adb0c00721ae267b7cfde4da7f71d3121cc2160c9fc00c0c89f8c53c8a", 25843},
}
