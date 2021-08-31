Proxy transform rules:

| Client protocol type  | Proxy type  | Headers | Meta |
| ------------- | ------------- | -------------- | -------------- |
| HTTP  | HTTP  |  In request headers |    No meta  |
| GRPC  | GRPC  |  No headers  |  In request meta  |
| HTTP  | GRPC  |  In request meta  |    No meta  |
| GRPC  | HTTP  |  No headers  |  In request headers  |
