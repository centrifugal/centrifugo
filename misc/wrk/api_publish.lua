-- ./centrifugo --api_insecure
-- wrk -s api_publish.lua http://localhost:8000/api -d 10s -t 10 -c 40
wrk.method = "POST"
wrk.body   = '{"method": "publish", "params": {"channel": "index", "data": 1}}'
wrk.headers["Content-Type"] = "application/json"
