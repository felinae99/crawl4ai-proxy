# crawl4ai OpenWebUI proxy

This simple proxy server can be run in a docker container to let an [OpenWebUI](https://github.com/open-webui/open-webui) instance interact with a [crawl4ai](https://github.com/unclecode/crawl4ai) instance.
This makes OpenWebUI's web search feature a lot faster and way more usable without paying for an API service. 🎉

## Usage

Given a `compose.yml` file that looks something like this:

```
services:
    crawl4ai-proxy:
        image: ghcr.io/felinae99/crawl4ai-proxy:latest
        environment:
            - LISTEN_PORT=8000
            - CRAWL4AI_ENDPOINT=http://crawl4ai:11235/crawl
            # API Token for crawl4ai authentication (0.9.x+)
            - CRAWL4AI_API_TOKEN=your_crawl4ai_api_token
        networks:
            - openwebui

    openwebui:
        image: ghcr.io/open-webui/open-webui:ollama
        ports:
            - "8080:8080"
        deploy:
            resources:
                reservations:
                    devices:
                        - driver: nvidia
                          count: all
                          capabilities: [gpu]
        networks:
            - openwebui

    crawl4ai:
        image: unclecode/crawl4ai:latest
        shm_size: 2g
        environment:
            # Must match CRAWL4AI_API_TOKEN in crawl4ai-proxy
            - CRAWL4AI_API_TOKEN=your_crawl4ai_api_token
            # Required for 0.9.x+: disable strict SSRF and TLS checks to prevent false-positive blocks
            - CRAWL4AI_ALLOW_INTERNAL_URLS=true
            - CRAWL4AI_ALLOW_INSECURE_TLS=true
        networks:
            - openwebui

networks:
    openwebui:
        external: false
```

Run `docker compose up -d`, visit `localhost:8080` in a browser, navigate to `Admin Panel -> Settings -> Web Search` and under the "Loader" section, set:

- **Web Loader Engine**: `external`
- **External Web Loader URL**: `http://crawl4ai-proxy:8000/crawl`
- **External Web Loader API Key**: `*` *(doesn't matter, but is a required field in OpenWebUI)*


Forked from [lennyerik/crawl4ai-proxy](https://github.com/lennyerik/crawl4ai-proxy).

Updated to support Crawl4AI 0.9.x authentication (`CRAWL4AI_API_TOKEN`) and proper request payload handling.
