#!/bin/sh
# Builds Timestamps.app for Apple silicon and Intel Macs into ./build and
# zips it as Timestamps-<version>-macos.zip, the name the in-app updater
# looks for in a release. Run on a Mac with Go and the Xcode command line
# tools.
set -eu
cd "$(dirname "$0")"

export CGO_ENABLED=1 MACOSX_DEPLOYMENT_TARGET=11.3
version=$(sed -n 's/^const Version = "\(.*\)"$/\1/p' internal/server/server.go)
app=build/Timestamps.app
zip=build/Timestamps-$version-macos.zip

rm -rf "$app" build/Timestamps-*-macos.zip
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
for arch in arm64 amd64; do
	GOARCH=$arch go build -trimpath -ldflags "-s -w" -o "build/timestamps-$arch" .
done
lipo -create -output "$app/Contents/MacOS/Timestamps" build/timestamps-arm64 build/timestamps-amd64
rm build/timestamps-arm64 build/timestamps-amd64

sed "s/__VERSION__/$version/g" assets/Info.plist > "$app/Contents/Info.plist"
cp assets/Timestamps.icns "$app/Contents/Resources/"
cp -R assets/zh-Hans.lproj "$app/Contents/Resources/"
plutil -lint "$app/Contents/Info.plist"

# Ad hoc signature: Apple silicon runs only signed code. Not notarized, so
# Gatekeeper asks before the first start (see docs/macos.md).
codesign --force --sign - "$app"
codesign --verify --verbose "$app"

ditto -c -k --keepParent "$app" "$zip"
echo "Built $app $version ($(lipo -archs "$app/Contents/MacOS/Timestamps")) and $zip"
