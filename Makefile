.PHONY: check build test vet fmt-check race test-32 fuzz bench corpus-download codepages-download codepages test-external oracle-download test-oracle test-render rendercheck-vet

check: fmt-check build test vet race test-32

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

fmt-check:
	@test -z "$$(gofmt -l $$(git ls-files --cached --others --exclude-standard '*.go'))"

race:
	go test -race ./...

test-32:
	GOARCH=386 go test ./...

fuzz:
	go test -run='^$$' -fuzz=FuzzWalk -fuzztime=30s -parallel=2 .
	go test -run='^$$' -fuzz=FuzzDecode -fuzztime=30s -parallel=2 .
	go test -run='^$$' -fuzz=FuzzDIB -fuzztime=30s -parallel=2 .
	go test -run='^$$' -fuzz=FuzzStream -fuzztime=30s -parallel=2 .
	go test -run='^$$' -fuzz=FuzzColorTransform -fuzztime=30s -parallel=2 .
	go test -run='^$$' -fuzz=FuzzTIFF -fuzztime=30s -parallel=2 .
	go test -run='^$$' -fuzz=FuzzPlay -fuzztime=30s -parallel=2 .

bench:
	go test -run='^$$' -bench=. -benchmem .

corpus-download:
	go run ./internal/corpusfetch

codepages-download:
	go run ./internal/corpusfetch -codepages

codepages: codepages-download
	go run ./internal/codepagegen

test-external: corpus-download codepages-download
	GOWEMF_EXTERNAL=1 go test -run='TestExternalCorpus|TestCodePageTables' -count=1 -v .

oracle-download:
	go run ./internal/corpusfetch -oracle

test-oracle: corpus-download oracle-download
	javac -cp '.external/oracle/*' -d .external/oracle tools/POIRecordDump.java
	GOWEMF_ORACLE=1 go test -run=TestPOIOracle -count=1 -v .

test-render: oracle-download
	javac -cp '.external/oracle/*' -d .external/oracle tools/POIBitmapRender.java
	GOWEMF_RENDER=1 go test -run=TestLibreOfficeRenderOracle -count=1 -v .
	cd rendercheck && GOWEMF_RENDER=1 go test -run=TestLibreOfficeText -count=1 -v .

rendercheck-vet:
	cd rendercheck && go vet ./...
