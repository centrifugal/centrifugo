## Generate HTTP API libraries from swagger spec: 

```
git clone --depth 1 -b master https://github.com/swagger-api/swagger-codegen.git
cd swagger-codegen
cp /path/to/api.swagger.json ./api.swagger.json
```

### Go library:

```
./run-in-docker.sh generate -i api.swagger.json -l go -o /gen/out/go-centrifugo -DpackageName=centrifugo
ls out/go-centrifugo
```

Then:

```go
package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"test_project/centrifugo"
)

func main() {
	httpClient := &http.Client{Transport: &http.Transport{
		MaxIdleConnsPerHost: 100,
	}, Timeout: time.Second}
	client := centrifugo.NewAPIClient(&centrifugo.Configuration{
		BasePath:      "http://localhost:8000/api",
		DefaultHeader: map[string]string{"Authorization": "apikey "},
		HTTPClient:    httpClient,
	})
	reply, resp, err := client.PublicationApi.CentrifugoApiPublish(context.Background(), centrifugo.PublishRequest{
		Channel: "test",
		Data:    map[string]string{},
	})
	if err != nil {
		panic(err)
	}
	if resp.StatusCode != 200 {
		panic(resp.StatusCode)
	}
	if reply.Error_ != nil {
		panic(reply.Error_.Message)
	}
	fmt.Println("ok")
}
```
