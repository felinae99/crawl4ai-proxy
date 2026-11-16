package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net/http"
    "os"
    "strconv"
    "strings"
    "time"
)

var (
    LISTEN_IP                  string = ""
    LISTEN_PORT                int    = 8000
    CRAWL4AI_ENDPOINT                 = "http://crawl4ai:11235/crawl"
    CRAWL4AI_API_TOKEN                = ""
    CRAWL4AI_EXCLUDED_TAGS     []string
    CRAWL4AI_USE_PRUNING       bool   = false
    CRAWL4AI_WORD_THRESHOLD    int    = 10
    CRAWL4AI_PAGE_TIMEOUT      int    = 15000
    CRAWL4AI_WAIT_FOR          string = "domcontentloaded"
)

func ReadEnvironment() {
    if portStr := os.Getenv("LISTEN_PORT"); portStr != "" {
        if port, err := strconv.Atoi(portStr); err == nil {
            LISTEN_PORT = port
        }
    }
    if ip := os.Getenv("LISTEN_IP"); ip != "" {
        LISTEN_IP = ip
    }
    if endpoint := os.Getenv("CRAWL4AI_ENDPOINT"); endpoint != "" {
        CRAWL4AI_ENDPOINT = endpoint
    }
    // 支持读取 API Token
    if token := os.Getenv("CRAWL4AI_API_TOKEN"); token != "" {
        CRAWL4AI_API_TOKEN = token
    } else if token := os.Getenv("CRAWL4AI_TOKEN"); token != "" {
        CRAWL4AI_API_TOKEN = token
    }

    // 读取正文修剪与过滤配置
    if tags := os.Getenv("CRAWL4AI_EXCLUDED_TAGS"); tags != "" {
        for _, tag := range strings.Split(tags, ",") {
            trimmed := strings.TrimSpace(tag)
            if trimmed != "" {
                CRAWL4AI_EXCLUDED_TAGS = append(CRAWL4AI_EXCLUDED_TAGS, trimmed)
            }
        }
    }
    if pruning := os.Getenv("CRAWL4AI_USE_PRUNING"); pruning != "" {
        CRAWL4AI_USE_PRUNING = (strings.ToLower(pruning) == "true" || pruning == "1")
    }
    if thresholdStr := os.Getenv("CRAWL4AI_WORD_COUNT_THRESHOLD"); thresholdStr != "" {
        if th, err := strconv.Atoi(thresholdStr); err == nil {
            CRAWL4AI_WORD_THRESHOLD = th
        }
    }
    if timeoutStr := os.Getenv("CRAWL4AI_PAGE_TIMEOUT"); timeoutStr != "" {
        if to, err := strconv.Atoi(timeoutStr); err == nil {
            CRAWL4AI_PAGE_TIMEOUT = to
        }
    }
    if waitFor := os.Getenv("CRAWL4AI_WAIT_FOR"); waitFor != "" {
        CRAWL4AI_WAIT_FOR = waitFor
    }
}

// 接收来自 OpenWebUI 的请求结构
type Request struct {
    Urls []string `json:"urls"`
}

// 发送给 Crawl4AI 0.9.x 的请求结构
type Crawl4AIRequestPayload struct {
    Urls            []string               `json:"urls"`
    CrawlerParams   map[string]interface{} `json:"crawler_params,omitempty"`
    ExtraParameters map[string]interface{} `json:"extra,omitempty"`
}

type SuccessResponseItem struct {
    PageContent string            `json:"page_content"`
    Metadata    map[string]string `json:"metadata"`
}

type SuccessResponse []SuccessResponseItem

type ErrorResponse struct {
    ErrorName string `json:"error"`
    Detail    string `json:"detail,omitempty"`
}

// Crawl4AI 响应结构解析
type CrawlResponse struct {
    Results []struct {
        Url      string `json:"url"`
        Markdown struct {
            RawMarkdown string `json:"raw_markdown"`
        } `json:"markdown"`
        // 兼容部分版本中直接返回 markdown 字符串的情况
        MarkdownStr string            `json:"markdown_v2,omitempty"`
        Metadata    map[string]string `json:"metadata"`
    } `json:"results"`
}

func errorResponseFromError(name string, err error) ErrorResponse {
    return ErrorResponse{
        ErrorName: name,
        Detail:    err.Error(),
    }
}

func jsonEncodeInfallible(object any) []byte {
    encoded, err := json.Marshal(object)
    if err != nil {
        panic(err)
    }
    return encoded
}

func CrawlEndpoint(response http.ResponseWriter, request *http.Request) {
    response.Header().Set("Content-Type", "application/json")
    if request.Method != "POST" {
        response.WriteHeader(405)
        response.Write(jsonEncodeInfallible(ErrorResponse{ErrorName: "method not allowed"}))
        log.Printf("405 method not allowed :: %s\n", request.RemoteAddr)
        return
    }

    if !strings.HasPrefix(request.Header.Get("Content-Type"), "application/json") {
        response.WriteHeader(400)
        response.Write(jsonEncodeInfallible(ErrorResponse{ErrorName: "content type must be application/json"}))
        log.Printf("400 invalid content type :: %s\n", request.RemoteAddr)
        return
    }

    var requestData Request
    err := json.NewDecoder(request.Body).Decode(&requestData)
    if err != nil {
        response.WriteHeader(400)
        response.Write(jsonEncodeInfallible(errorResponseFromError("invalid json", err)))
        log.Printf("400 invalid json :: %s\n", request.RemoteAddr)
        return
    }

    log.Printf("Request to crawl %v from %s\n", requestData.Urls, request.RemoteAddr)

    // 组装 Crawl4AI 0.9.x 规范的参数
    crawlerParams := map[string]interface{}{
        "page_timeout": CRAWL4AI_PAGE_TIMEOUT,
        "wait_until":   CRAWL4AI_WAIT_FOR,
    }
    if len(CRAWL4AI_EXCLUDED_TAGS) > 0 {
        crawlerParams["excluded_tags"] = CRAWL4AI_EXCLUDED_TAGS
    }

    extraParams := map[string]interface{}{
        "word_count_threshold": CRAWL4AI_WORD_THRESHOLD,
    }
    if CRAWL4AI_USE_PRUNING {
        extraParams["content_filter"] = map[string]interface{}{
            "type":               "pruning",
            "min_word_threshold": CRAWL4AI_WORD_THRESHOLD,
        }
    }

    c4aPayload := Crawl4AIRequestPayload{
        Urls:            requestData.Urls,
        CrawlerParams:   crawlerParams,
        ExtraParameters: extraParams,
    }

    reqBody := jsonEncodeInfallible(c4aPayload)
    req, err := http.NewRequest("POST", CRAWL4AI_ENDPOINT, bytes.NewReader(reqBody))
    if err != nil {
        panic(err)
    }

    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Accept", "application/json")

    // 注入 Authorization 鉴权头
    if CRAWL4AI_API_TOKEN != "" {
        req.Header.Set("Authorization", "Bearer "+CRAWL4AI_API_TOKEN)
    }

    client := &http.Client{
        Timeout: time.Duration(CRAWL4AI_PAGE_TIMEOUT+10000) * time.Millisecond,
    }

    crawlResponse, err := client.Do(req)
    if err != nil {
        response.WriteHeader(502)
        response.Write(jsonEncodeInfallible(ErrorResponse{ErrorName: "bad gateway", Detail: err.Error()}))
        log.Printf("502 bad gateway (network error: %v) :: %s\n", err, request.RemoteAddr)
        return
    }
    defer crawlResponse.Body.Close()

    if crawlResponse.StatusCode != 200 {
        respBody, _ := io.ReadAll(crawlResponse.Body)
        response.WriteHeader(502)
        response.Write(jsonEncodeInfallible(ErrorResponse{
            ErrorName: "bad gateway",
            Detail:    fmt.Sprintf("crawl4ai returned status %d", crawlResponse.StatusCode),
        }))
        log.Printf("502 bad gateway (crawl4ai status: %d, body: %s) :: %s\n", crawlResponse.StatusCode, string(respBody), request.RemoteAddr)
        return
    }

    var crawlData CrawlResponse
    err = json.NewDecoder(crawlResponse.Body).Decode(&crawlData)
    if err != nil {
        response.WriteHeader(502)
        response.Write(jsonEncodeInfallible(ErrorResponse{ErrorName: "bad gateway", Detail: "invalid json received from crawl api"}))
        log.Printf("502 bad gateway - invalid json from crawl api :: %s\n", request.RemoteAddr)
        return
    }

    ret := SuccessResponse{}
    for _, result := range crawlData.Results {
        if result.Metadata == nil {
            result.Metadata = map[string]string{}
        }
        for key, value := range result.Metadata {
            if value == "" {
                delete(result.Metadata, key)
            }
        }
        result.Metadata["source"] = result.Url

        content := result.Markdown.RawMarkdown
        if content == "" && result.MarkdownStr != "" {
            content = result.MarkdownStr
        }

        ret = append(ret, SuccessResponseItem{
            PageContent: content,
            Metadata:    result.Metadata,
        })
    }

    response.WriteHeader(200)
    response.Write(jsonEncodeInfallible(ret))
    log.Printf("200 OK :: %s\n", request.RemoteAddr)
}

func main() {
    ReadEnvironment()
    http.HandleFunc("/crawl", CrawlEndpoint)
    listenAddress := fmt.Sprintf("%s:%d", LISTEN_IP, LISTEN_PORT)
    log.Printf("Listening on %s, forwarding to %s (Auth: %v)\n", listenAddress, CRAWL4AI_ENDPOINT, CRAWL4AI_API_TOKEN != "")
    err := http.ListenAndServe(listenAddress, nil)
    if err != nil {
        log.Println(err)
    }
}
