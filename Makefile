CMDS = goxdr
CLEANFILES = .*~ *~ */*~ goxdr
BUILT_SOURCES = rpc/rpc_msg.go xdr/boilerplate.go rpc/prot_test.go

all: build man

build: cmd/goxdr/goxdr $(BUILT_SOURCES) always
	go build ./xdr ./rpc

rpc/rpc_msg.go: rpc/rpc_msg.x goxdr
	./goxdr -enum-comments -p $$(dirname $@) -o $@ rpc/rpc_msg.x

rpc/prot_test.go: rpc/prot_test.x goxdr
	./goxdr -enum-comments -p rpc_test -o $@ rpc/prot_test.x

xdr/boilerplate.go: goxdr
	./goxdr -B -p $$(dirname $@) -o $@

cmd/goxdr/goxdr: always go.mod
	cd cmd/goxdr && $(MAKE)

goxdr: cmd/goxdr/goxdr
	@set -x;
	if test $$(go env GOARCH) == $$(go env GOHOSTARCH); then \
		cp cmd/goxdr/goxdr .; \
	else \
		GOARCH=$$(go env GOHOSTARCH) go build -o $@ ./cmd/goxdr/; \
	fi

go.mod: $(MAKEFILE_LIST)
	echo 'module github.com/xdrpp/goxdr' > go.mod

depend: always
	cd / && go install golang.org/x/tools/cmd/goyacc@latest

RECURSE = for dir in $(CMDS); do cd cmd/$$dir && $(MAKE) $@; done

test: always $(BUILT_SOURCES)
	go test -v ./rpc
	$(RECURSE)

clean: always
	rm -f $(CLEANFILES)
	rm -rf goroot gh-pages
	$(RECURSE)

maintainer-clean: always
	rm -f $(CLEANFILES) $(BUILT_SOURCES) go.mod go.sum
	rm -rf goroot gh-pages
	$(RECURSE)

install uninstall man: always
	$(RECURSE)

built_sources: $(BUILT_SOURCES) go.mod
	rm -f $@
	for file in $(BUILT_SOURCES); do \
		echo $$file >> $@; \
	done
	$(RECURSE)

go1: always
	./make-go1

gh-pages: always
	./make-gh-pages

always:
	@:
.PHONY: always
