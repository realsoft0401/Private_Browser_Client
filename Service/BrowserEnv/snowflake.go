package BrowserEnv

import (
	"strconv"
	"sync"
	"time"
)

var idGen = newSnowflakeGenerator(1)

type snowflakeGenerator struct {
	mu       sync.Mutex
	epochMS  int64
	workerID int64
	lastMS   int64
	sequence int64
}

// newSnowflakeGenerator 创建本地雪花 ID 生成器。
//
// 设计来源：
// - 环境包第一版由边缘服务本机生成 envId，不依赖中心服务端；
// - snowflakeId 会写入 profile，成为 envId 的唯一编码部分；
// - workerID 暂定为 1，未来如果中心服务端下发边缘分片 ID，只需要替换这里的 workerID 来源。
func newSnowflakeGenerator(workerID int64) *snowflakeGenerator {
	return &snowflakeGenerator{
		epochMS:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli(),
		workerID: workerID & 0x3ff,
	}
}

// Next 生成一个递增且足够唯一的雪花 ID 字符串。
//
// 这里返回字符串，是因为 envId、目录名、profile 都把它当作外部稳定编码，
// 避免前端或 JSON 解析环境因为大整数精度导致 ID 变化。
func (g *snowflakeGenerator) Next() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	nowMS := time.Now().UnixMilli()
	if nowMS < g.lastMS {
		nowMS = g.lastMS
	}
	if nowMS == g.lastMS {
		g.sequence = (g.sequence + 1) & 0xfff
		if g.sequence == 0 {
			for nowMS <= g.lastMS {
				nowMS = time.Now().UnixMilli()
			}
		}
	} else {
		g.sequence = 0
	}
	g.lastMS = nowMS

	id := ((nowMS - g.epochMS) << 22) | (g.workerID << 12) | g.sequence
	return strconv.FormatInt(id, 10)
}
