default: build

build:
	go build prequel.go

install:
	mkdir -p ${DESTDIR}/usr/bin
	cp prequel ${DESTDIR}/usr/bin/prequel

install-dev:
	mkdir -p ${DESTDIR}/usr/bin
	ln -s `pwd`/prequel ${DESTDIR}/usr/bin/prequel

uninstall:
	rm ${DESTDIR}/usr/bin/prequel

clean:
	rm prequel
