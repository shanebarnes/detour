package main

import (
    "bytes"
    "crypto/md5"
    "encoding/base64"
    "io/ioutil"
    "net"
    "net/http"
    "strconv"
    "sync/atomic"
    "time"

    "github.com/twinj/uuid"
)

const azcopyBlockSize int64 = 4 * 1024 * 1024

type CacheEntry struct {
    block         []byte
    contentLength   int64
    contentMd5      string
}

type AzureBlobContext struct {
    blockCache    []CacheEntry
    contentLength   int64
    tag             string
}

type MilestoneOp func(*ShortcutAzureBlob, Role, []byte)(bool, int, error)

type ShortcutAzureBlob struct {
    ctxt          []AzureBlobContext
    mlst          []MilestoneOp
    requestCount    int32
    responseCount   int32
    impl            ShortcutImpl
}

func (s *ShortcutAzureBlob) New(route int, client net.Conn, server net.Conn) error {
    s.newContext(route, Client)
    s.newContext(route, Server)

    s.mlst = append(s.mlst, (*ShortcutAzureBlob).isMilestoneRequest)
    s.mlst = append(s.mlst, (*ShortcutAzureBlob).isMilestoneResponse)
    s.mlst = append(s.mlst, (*ShortcutAzureBlob).isMilestoneContinue)
    s.mlst = append(s.mlst, (*ShortcutAzureBlob).isMilestoneNone)

    return s.impl.New(route, client, server)
}

func (s *ShortcutAzureBlob) newContext(route int, role Role) {
    s.ctxt = append(s.ctxt, AzureBlobContext{})

    if int(role) == len(s.ctxt) - 1 {
        number := "{R" + strconv.Itoa(route) + "}"

        switch role {
        case Client:
            s.ctxt[role].tag = number + " [AZURE CLIENT]"
        case Server:
            s.ctxt[role].tag = number + " [AZURE SERVER]"
        default:
            s.ctxt[role].tag = number + " [AZURE ?ROLE?]"
        }

        s.ctxt[role].blockCache = make([]CacheEntry, 0)
        s.ctxt[role].contentLength = 0
        atomic.StoreInt32(&s.requestCount, 0)
        atomic.StoreInt32(&s.responseCount, 0)
    }
}

func (s *ShortcutAzureBlob) Take(role Role, buffer []byte) (int, error) {
    var takenFlag  bool = false
    var takenBytes int = 0
    var takenError error = nil

    // @todo Need to keep the underlying TCP connection to the server open until
    //       all blocks in the cache are acknowledged even if the client has
    //       closed its connection.
    // @todo Cache should include the original request/response header (not just
    //       body), which would be necessary for retransmission.
    // @todo Responses should look for a match in cache against MD5.
    for i := range s.mlst {
        if takenFlag, takenBytes, takenError = s.mlst[i](s, role, buffer); takenFlag {
            break
        }
    }

    return takenBytes, takenError
}

func (s *ShortcutAzureBlob) isMilestoneRequest(role Role, buffer []byte) (bool, int, error) {
    foundRequest := false
    var takenBytes int = 0
    var takenErr error = nil

    if s.ctxt[role].contentLength == 0 {
        payload := string(buffer)
        if request, reqSize := GetHttpRequest(&payload); request != nil {
            foundRequest = true

            //_logger.Println("\n\n", request, "\n\n")
            //_logger.Println(s.ctxt[role].tag, "Request Method:", request.Method, "Size:", request.ContentLength)

            // Ignore zero content lengths or content lengths of unknown size
            // (-1).
            // To wait for the last block to go all the way to the Azure Blob
            // server only requests of content lengths equal to 4 MiB will be
            // cached so that the last block request/response will prevent the
            // AzCopy from closing connections prematurely.
            if role == Client && request.Method == "PUT" && request.ContentLength == azcopyBlockSize {
                s.ctxt[role].contentLength = request.ContentLength
                atomic.AddInt32(&s.requestCount, 1)
                s.cachePushBack(role, request.ContentLength, request.Header.Get("Content-MD5"))

                if reqSize < len(buffer) {
                    s.blockPushBack(role, len(s.ctxt[role].blockCache) - 1, buffer[reqSize:])
                    s.updateContentLeft(role, int64(len(buffer) - reqSize))
                    if s.ctxt[role].contentLength == 0 {
                        s.sendResponse(role, len(s.ctxt[role].blockCache) - 1)
                    }

                }
            }

           // Remove acknowledged blocks from cache
           for atomic.LoadInt32(&s.responseCount) > 0 {
               s.cachePopFront(role)
               atomic.AddInt32(&s.requestCount, -1)
               atomic.AddInt32(&s.responseCount, -1)
           }
        }
    }

    if foundRequest {
        takenBytes, takenErr = s.impl.Take(role, buffer)
    }

    return foundRequest, takenBytes, takenErr
}

func (s *ShortcutAzureBlob) isMilestoneResponse(role Role, buffer []byte) (bool, int, error) {
    dropResponse := false
    foundResponse := false
    var takenBytes int = 0
    var takenErr error = nil

    if s.ctxt[role].contentLength == 0 {
        payload := string(buffer)
        if r, _ := GetHttpResponse(&payload); r != nil {
            foundResponse = true

            //_logger.Println("\n\n", r, "\n\n")
            //_logger.Println(s.ctxt[role].tag, "Response Status:", r.Status)

            if role == Server && r.StatusCode == http.StatusCreated {
                if s.impl.use && atomic.LoadInt32(&s.requestCount) > atomic.LoadInt32(&s.responseCount) {
                    atomic.AddInt32(&s.responseCount, 1)
                    dropResponse = true
                    takenBytes = len(buffer)
                    //_logger.Println(s.ctxt[role].tag, "Dropping response to client")
                }
            }
        }
    }

    if foundResponse && !dropResponse {
        takenBytes, takenErr = s.impl.Take(role, buffer)
    }

    return foundResponse, takenBytes, takenErr
}

func (s *ShortcutAzureBlob) isMilestoneContinue(role Role, buffer []byte) (bool, int, error) {
    foundContinue := false
    var takenBytes int = 0
    var takenErr error = nil

    if role == Client && s.ctxt[role].contentLength != 0 {
        foundContinue = true
        cacheId := len(s.ctxt[role].blockCache) - 1
        s.blockPushBack(role, cacheId, buffer)
        s.updateContentLeft(role, int64(len(buffer)))

        if s.ctxt[role].contentLength == 0 {
            s.sendResponse(role, cacheId)
        }
    }

    if foundContinue {
        takenBytes, takenErr = s.impl.Take(role, buffer)
    }

    return foundContinue, takenBytes, takenErr
}

func (s *ShortcutAzureBlob) isMilestoneNone(role Role, buffer []byte) (bool, int, error) {
    foundNone := true
    takenBytes, takenError := s.impl.Take(role, buffer)

    return foundNone, takenBytes, takenError
}

func (s *ShortcutAzureBlob) cachePushBack(role Role, blockLength int64, blockMd5 string) {
    s.ctxt[role].blockCache = append(s.ctxt[role].blockCache, CacheEntry{ contentLength: blockLength, contentMd5: blockMd5 })
    //_logger.Println(s.ctxt[role].tag, "Increased cache size to", len(s.ctxt[role].blockCache))
}

func (s *ShortcutAzureBlob) cachePopFront(role Role) {
    //blockSize := len(s.ctxt[role].blockCache[0])

    if len(s.ctxt[role].blockCache) > 1 {
        s.ctxt[role].blockCache = s.ctxt[role].blockCache[1:]
    } else {
        s.ctxt[role].blockCache = s.ctxt[role].blockCache[:0]
    }

    //_logger.Println(s.ctxt[role].tag, "Decreased cache size to", len(s.ctxt[role].blockCache), "( Removed block of size", blockSize, ")")
}

func (s *ShortcutAzureBlob) blockPushBack(role Role, cacheId int, chunk []byte) {
    if cacheId >= 0 && cacheId < len(s.ctxt[role].blockCache) {
        //s.ctxt[role].blockCache[cacheId].block = append(s.ctxt[role].blockCache[cacheId].block, chunk...)
        //_logger.Println(s.ctxt[role].tag, "Block", cacheId, "size is now:", len(s.ctxt[role].blockCache[cacheId]))
    } else {
        _logger.Println(s.ctxt[role].tag, "Invalid cache index", cacheId)
    }
}

func (s *ShortcutAzureBlob) updateContentLeft(role Role, contentLength int64) { // Update "distance" left to travel
    if contentLength > s.ctxt[role].contentLength {
        _logger.Println(s.ctxt[role].tag, "content length mismatch (", contentLength, ">", s.ctxt[role].contentLength, ")")
        s.ctxt[role].contentLength = 0
    } else {
        s.ctxt[role].contentLength = s.ctxt[role].contentLength - contentLength
    }
}

func (s *ShortcutAzureBlob) sendResponse(role Role, blockId int) {
    body := ""
    rsp := &http.Response {
        Status:        "201 Created",
        StatusCode:    http.StatusCreated,
        Proto:         "HTTP/1.1",
        ProtoMajor:    1,
        ProtoMinor:    1,
        Body:          ioutil.NopCloser(bytes.NewBufferString(body)),
        ContentLength: int64(len(body)),
        Request:       nil,
        Header:        make(http.Header, 0),
    }

    // @note Preserve lower-case header key names by avoiding header setters.
    // E.g., rsp.Header.Set("x-ms-version", "2016-05-31") => X-Ms-Version
    rsp.TransferEncoding = []string{"chunked"}
    rsp.Header["Content-MD5"] = []string{s.ctxt[role].blockCache[blockId].contentMd5}
    rsp.Header["Server"] = []string{"Windows-Azure-Blob/1.0 Microsoft-HTTPAPI/2.0"}
    rsp.Header["x-ms-request-id"] = []string{createServerHeaderRequestId()}
    rsp.Header["x-ms-version"] = []string{"2016-05-31"} // @todo Use version from original client request?
    rsp.Header["x-ms-request-server-encrypted"] = []string{"true"}
    rsp.Header["Date"] = []string{createServerHeaderDate()}

    rspBuf := bytes.NewBuffer(nil)
    rsp.Write(rspBuf)

    if s.impl.use {
        //_logger.Println(s.ctxt[role].tag, " Sending fast response to client for block of size ", s.ctxt[role].blockCache[blockId].contentLength)
        //_logger.Println(rsp, "\n\n")
        s.impl.side[role].Write(rspBuf.Bytes())
    }
}

func createServerHeaderContentMd5(block []byte) string {
    hasher := md5.New()
    hasher.Write(block)

    return base64.StdEncoding.EncodeToString(hasher.Sum(nil))
}

func createServerHeaderDate() string {
    now := (time.Now().UTC()).Format(time.RFC1123)

    // Replace incorrect "UTC" with "GMT"
    n := len(now)
    if n > 3 {
        now = now[:n-3]
        now = now + "GMT"
    }

    return now
}

func createServerHeaderRequestId() string {
    u1 := uuid.NewV1()
    return u1.String()
}
