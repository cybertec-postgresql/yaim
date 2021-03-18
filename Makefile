all: yaim

yaim: *.go */*.go
	go build -ldflags="-s -w" .

package: package-rpm package-deb

package-rpm: all
	nfpm pkg -f packaging/nfpm.yaml --target packaging/yaim.rpm

package-deb: all
	nfpm pkg -f packaging/nfpm.yaml --target packaging/yaim.deb

clean:
	rm -rf yaim
	rm -rf packaging/yaim.deb
	rm -rf packaging/yaim.rpm