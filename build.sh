#!/usr/bin/env bash

export COPYFILE_DISABLE=true

distFiles=(README.md LICENSE) # apart from the binary itself
version=$(git describe --always)

build() {
	go build -ldflags "-w -X main.Version $version" ./cmd/syncthing
}

prepare() {
	go run cmd/assets/assets.go gui > auto/gui.files.go
	go get -d
}

test() {
	go test ./...
}

tarDist() {
	name="$1"
	rm -rf "$name"
	mkdir -p "$name"
	cp syncthing "${distFiles[@]}" "$name"
	gpg -ab "$name/syncthing"
	tar zcvf "$name.tar.gz" "$name"
	rm -rf "$name"
}

zipDist() {
	name="$1"
	rm -rf "$name"
	mkdir -p "$name"
	cp syncthing.exe "${distFiles[@]}" "$name"
	gpg -ab "$name/syncthing.exe"
	zip -r "$name.zip" "$name"
	rm -rf "$name"
}

case "$1" in
	"")
		build
		;;

	tar)
		rm -f *.tar.gz *.zip
		prepare
		test || exit 1
		build

		eval $(go env)
		name="syncthing-$GOOS-$GOARCH-$version"

		tarDist "$name"
		;;

	all)
		rm -f *.tar.gz *.zip
		prepare
		test || exit 1

		export GOARM=7
		for os in darwin-amd64 linux-amd64 linux-arm freebsd-amd64 ; do
			export GOOS=${os%-*}
			export GOARCH=${os#*-}

			build

			name="syncthing-$os-$version"
			case $GOOS in
				windows)
					zipDist "$name"
					rm -f syncthing.exe
					;;
				*)
					tarDist "$name"
					rm -f syncthing
					;;
			esac
		done
		;;

	upload)
		tag=$(git describe)
		shopt -s nullglob
		for f in *.tar.gz *.zip *.asc ; do
			relup calmh/syncthing "$tag" "$f"
		done
		;;

	*)
		echo "Unknown build parameter $1"
		;;
esac
