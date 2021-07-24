-- CENTRIFUGO_HISTORY_SIZE=100 CENTRIFUGO_HISTORY_TTL=3600s ./centrifugo --api_insecure
-- for i in {1..100}; do curl -X POST http://localhost:8000/api --data '{"method": "publish", "params": {"channel": "index", "data": "hello"}}'; done
-- curl -X POST http://localhost:8000/api --data '{"method": "history", "params": {"channel": "index", "limit": -1}}' | python3 -m json.tool
-- wrk -s api_history.lua http://localhost:8000/api -d 2s -t 5 -c 40
wrk.method = "POST"
wrk.body   = '{"method": "history", "params": {"channel": "index", "limit": -1}}'
wrk.headers["Content-Type"] = "application/json"
