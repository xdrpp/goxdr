CMDS = goxdr
CLEANFILES = .*~ *~ */*~ goxdr
BUILT_SOURCES = rpcmsg/rpc_msg.go xdr/boilerplate.go

all: build man

build: cmd/goxdr/goxdr $(BUILT_SOURCES) always
	go build ./xdr ./rpcmsg

rpcmsg/rpc_msg.go: rpcmsg/rpc_msg.x goxdr
	./goxdr -enum-comments -p $$(dirname $@) -o $@ rpcmsg/rpc_msg.x

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
	cd / && go get -u golang.org/x/tools/cmd/goyacc

RECURSE = for dir in $(CMDS); do cd cmd/$$dir && $(MAKE) $@; done

test: always
	go test -v .
	$(RECURSE)

clean: always
	rm -f $(CLEANFILES)
	$(RECURSE)

maintainer-clean: always
	rm -f $(CLEANFILES) $(BUILT_SOURCES) go.mod go.sum
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
