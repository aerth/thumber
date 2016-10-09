#!/bin/sh
# aerth@riseup.net

HELP='Thumber Stress Tester
(Thumbnail server on :8081)

Usage:

./test.sh up
./test.sh home
./test.sh resize
./test.sh cache

Make sure to start the server with -swamped flag to disable rate limiting!'
if [ -z $1 ]; then
echo "$HELP"
exit 1
fi

// Visit localhost:8081, 3200 times
homeTest() {
  for i in {1..3200}; do
  curl localhost:8081 >& /dev/null
  curl localhost:8081 >& /dev/null
done
}

// Upload testdata/one.jpeg
uploadTest() {
  for i in {1..3200}; do
curl --form file=@testdata/one.jpeg localhost:8081/upload -v
done
# then something like
# for i in $(ls); do diff __pickone__ $i; done

}

// Send 3200 different width requests
resizeTest() {
curl localhost:8081/{1..3200}/0/90238293a2d966b44304981634a3686281980b853a5ge9447219792937ddf9674a3d4010130046261c566591233a61e92b5f
}

// 4 of the same requests
cacheTest() {
curl localhost:8081/100/100/90238293a2d966b44304981634a3686281980b853a5ge9447219792937ddf9674a3d4010130046261c566591233a61e92b5f
}

case "$@" in
'home') #
  homeTest
  ;;
'up') #
  uploadTest
  ;;
'resize') # test 1, swamp the server with requests for different widths
  resizeTest
  ;;
'cache') # test 1, swamp the server with requests for different widths
  cacheTest;cacheTest;cacheTest;cacheTest;
  ;;

*) # default
  echo $HELP
esac
