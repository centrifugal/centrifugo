set -e

cd internal/apiproto
bash generate.sh
cd -

cd internal/apiproto/swagger
bash generate_swagger.sh
cd -

cd internal/proxyproto
bash generate.sh
cd -

cd internal/unigrpc/unistream
bash generate.sh
cd -
