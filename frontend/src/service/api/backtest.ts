import { request } from '../request'

/** 运行策略回测 (空条件 = 全市场基准) */
export function runBacktest(data: Api.Stock.BacktestRequest) {
  return request<Api.Stock.BacktestResponse>({
    url: '/api/stock/backtest',
    method: 'post',
    data
  })
}
