all: yaim

yaim: *.go */*.go
	go build -ldflags="-s -w" .

package: package-rpm package-deb

package-rpm: all
	nfpm pkg -f packaging/nfpm.yaml --target packaging --packager rpm
	# podman run --rm -it -v $(PWD):/tmp/pkg:Z -w /tmp/pkg goreleaser/nfpm package --config /tmp/pkg/packaging/nfpm.yaml --target /tmp/pkg/packaging --packager rpm

package-deb: all
	nfpm pkg -f packaging/nfpm.yaml --target packaging --packager deb
	# podman run --rm -it -v $(PWD):/tmp/pkg:Z -w /tmp/pkg goreleaser/nfpm package --config /tmp/pkg/packaging/nfpm.yaml --target /tmp/pkg/packaging --packager deb

clean:
	rm -rf yaim
	rm -rf packaging/*.deb
	rm -rf packaging/*.rpm