package Task

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"private_browser_client/Pkg/HttpResponse"
	common "private_browser_client/Repository/Common"
)

// GetDetail 是统一任务详情查询入口。
//
// 它只返回当前进程内任务摘要，不返回 SSE 流；如果调用方需要过程事件，仍然必须继续订阅
// `/api/v1/edge/tasks/{taskId}/events`。
func GetDetail(c *gin.Context) {
	result, err := GetService().GetDetail(c.Param("taskId"))
	if err != nil {
		if err == common.ErrNotFound {
			HttpResponse.ResponseError(c, HttpResponse.CodeNotFound)
			return
		}
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeServerBusy, err.Error())
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// SubscribeEvents 是统一 SSE 任务订阅入口。
//
// 这里故意只做“协议转发”和最小错误映射：
// - 不创建任务；
// - 不决定任务成功失败；
// - 只把现有任务事件按统一 SSE 协议输出给前端或 Node Server。
func SubscribeEvents(c *gin.Context) {
	snapshot, stream, cancel, err := GetService().Subscribe(c.Param("taskId"))
	if err != nil {
		if err == common.ErrNotFound {
			c.JSON(http.StatusOK, gin.H{
				"code":    1002,
				"message": "数据不存在",
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code":    1005,
			"message": err.Error(),
		})
		return
	}
	defer cancel()

	writer := c.Writer
	header := writer.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	for _, event := range snapshot.Events {
		if err = writeSSEEvent(writer, event); err != nil {
			return
		}
	}
	if snapshot.Done || stream == nil {
		return
	}

	notify := c.Request.Context().Done()
	for {
		select {
		case <-notify:
			return
		case event, ok := <-stream:
			if !ok {
				return
			}
			if err = writeSSEEvent(writer, event); err != nil {
				return
			}
		}
	}
}

func writeSSEEvent(writer gin.ResponseWriter, event any) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal task event failed: %w", err)
	}
	if _, err = fmt.Fprintf(writer, "event: %s\n", extractEventName(event)); err != nil {
		return err
	}
	if _, err = fmt.Fprintf(writer, "data: %s\n\n", body); err != nil {
		return err
	}
	writer.Flush()
	return nil
}

func extractEventName(event any) string {
	type eventNamer interface {
		GetEvent() string
	}
	if named, ok := event.(eventNamer); ok {
		return named.GetEvent()
	}
	return "message"
}
