curl http://localhost:8000/api --header "Content-Type: application/json" \
  --header "Authorization: apikey ${CENTRIFUGO_API_KEY}" \
  --request POST \
  --data '{"method": "rpc", "params": {"method": "getUserStatus", "params": {"users": ["'${CENTRIFUGO_USER}'"]}}}'
