## To set up:
##   brew install protoc-gen-go
## protoc executable is placed on the standard path after installing standard PB support

protoc --go_opt=Mnotehub.proto=./notelib --go_out=. notehub.proto
mv notelib/*.go .
rmdir notelib

## Download and unzip nanopb sdk into a folder, and point to it with the env var NANOPBSDK
## https://koti.kapsi.fi/jpa/nanopb/
## Then,
export NANOPBSDK=~/dev/nordic/external/nano-pb
${NANOPBSDK}/generator-bin/protoc -I./ -I${NANOPBSDK}/generator/proto/ --nanopb_out=./clang notehub.proto
