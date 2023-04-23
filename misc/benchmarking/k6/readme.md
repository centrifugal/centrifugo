A simple script which demonstrates benchmarking Centrifugo with [k6](https://k6.io/). You can use it as a starting point.

It connects to Centrifugo with JWT per user, subscribes to the `test` channel, waits some time and exits.

## Running test

Run Centrifugo with config like this:

```json
{
  "token_hmac_secret_key": "secret",
  "admin": true,
  "admin_password": "password",
  "admin_secret": "0453661f-8b95-4f75-ab49-cfb2e3b2024f",
  "api_key": "7d8cee29-f5c0-419e-ae70-760767b21f8e",
  "allow_subscribe_for_client": true,
  "allowed_origins": []
}
```

Then:

```
k6 run benchmark.js
```

Make sure [your system is tuned](https://centrifugal.dev/docs/server/infra_tuning) for dealing with many connections.
