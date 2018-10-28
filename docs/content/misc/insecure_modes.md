# Insecure modes

This chapter describes several insecure options that enable several insecure modes in Centrifugo.

### Insecure client connection

The boolean option `client_insecure` (default `false`) allows to connect to Centrifugo without JWT token. This means there is no user authentication involved. This mode can be useful to demo projects based on Centrifugo, personal projects or real-time application prototyping.

### Insecure API mode

This mode can be enabled using boolean option `api_insecure` (default `false`). When on there is no need to provide API key in HTTP requests. When using this mode everyone that has access to `/api` endpoint can send any command to server. Enabling this option can be reasonable if `/api` endpoint protected by firewall rules.

The option is also useful in development to simplify sending API commands to Centrifugo using CURL for example without specifying `Authorization` header in requests.

### Insecure admin mode

This mode can be enabled using boolean option `admin_insecure` (default `false`). When on there is no authentication in admin web interface. Again - this is not secure but can be justified if you protected admin interface by firewall rules or you want to use basic authentication for Centrifugo admin interface.
