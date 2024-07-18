set -e

if ! command -v ent &> /dev/null
then
    echo "ent could not be found"
    exit
fi

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
