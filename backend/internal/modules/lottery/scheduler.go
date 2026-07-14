package lottery

import (
	"context"
	"log"
	"time"
)

const lotterySchedulerInterval = time.Second

func (s *Service) StartScheduler(ctx context.Context) {
	go func() {
		s.schedulerTick(ctx)
		// 报名与开奖都向用户展示秒级倒计时，因此状态推进也必须保持秒级精度。
		// 相关查询均使用状态与时间索引，只扫描已到期活动，不随历史活动总量线性增长。
		ticker := time.NewTicker(lotterySchedulerInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.schedulerTick(ctx)
			}
		}
	}()
}

func (s *Service) schedulerTick(ctx context.Context) {
	defer func() {
		if value := recover(); value != nil {
			log.Printf("[lottery] scheduler recovered panic=%v", value)
		}
	}()
	s.RunSchedulerTick(ctx)
}
