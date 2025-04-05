package agent

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"
)

// HotToken 存储热门代币信息
type HotToken struct {
	Pair         string  `json:"pair"`
	Chain        string  `json:"chain"`
	Amm          string  `json:"amm"`
	TargetToken  string  `json:"target_token"`
	TokenSymbol  string  `json:"token0_symbol"`
	Volume15m    float64 `json:"volume_u_15m"` // 15分钟交易量
	VolumeUSD24h float64 `json:"volume_u_24h"`
	FlashAgent   Agent
}

// APIResponse API响应结构
type APIResponse struct {
	Status int `json:"status"`
	Data   struct {
		Total    int        `json:"total"`
		PageNO   int        `json:"pageNO"`
		PageSize int        `json:"pageSize"`
		Data     []HotToken `json:"data"`
	} `json:"data"`
}

// SolscanPoolResponse Solscan池响应结构
type SolscanPoolResponse struct {
	Success bool `json:"success"`
	Data    []struct {
		PoolID     string `json:"pool_id"`
		ProgramID  string `json:"program_id"`
		TokensInfo []struct {
			Token        string `json:"token"`
			TokenAccount string `json:"token_account"`
		} `json:"tokens_info"`
		TotalTrades24h int64 `json:"total_trades_24h"`
		TotalVolume24h int64 `json:"total_volume_24h"`
	} `json:"data"`
	Metadata struct {
		Accounts map[string]struct {
			AccountAddress string   `json:"account_address"`
			AccountLabel   string   `json:"account_label"`
			AccountTags    []string `json:"account_tags"`
			AccountType    string   `json:"account_type"`
		} `json:"accounts"`
	} `json:"metadata"`
}

// TokenPoolsInfo 存储代币的池信息
type TokenPoolsInfo struct {
	TokenAddress   string
	TokenSymbol    string
	Volume15m      float64
	PumpPools      []string
	MeteoraLists   []string
	RaydiumPools   []string
	RaydiumCPPools []string
}

// HotTokensTracker 热门代币跟踪器
type HotTokensTracker struct {
	APIURL          string
	PollInterval    time.Duration
	HotTokens       []HotToken
	MevConfig       *Config
	AgentConfig     *FlashAgentConfig
	TokenPoolsInfos []TokenPoolsInfo // 存储所有代币的池信息
	Agent           *Agent
}

// NewHotTokensTracker 创建新的热门代币跟踪器
func NewHotTokensTracker(mevConfig *Config, agentConfig *FlashAgentConfig, agent *Agent) *HotTokensTracker {
	return &HotTokensTracker{
		APIURL:       "https://febweb002.com/v1api/v4/tokens/treasure/list",
		PollInterval: 45 * time.Minute,
		HotTokens:    []HotToken{},
		MevConfig:    mevConfig,
		AgentConfig:  agentConfig,
		Agent:        agent,
	}
}

// FetchHotTokens 获取30分钟内交易量最大的热门代币
func (h *HotTokensTracker) FetchHotTokens() error {
	// 构建请求URL和参数 - 获取更多数据然后按15分钟交易量排序
	url := fmt.Sprintf("%s?chain=solana&pageNO=1&pageSize=40&category=hot&refresh_total=0", h.APIURL)

	// 创建请求
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("创建请求失败: %v", err)
		return err
	}

	// 添加认证Token到请求头
	req.Header.Set("X-Auth", h.AgentConfig.Ave.Token)
	req.Header.Set("Accept", "application/json")

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("请求API失败: %v", err)
		return err
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != 200 {
		log.Printf("API请求失败，状态码: %d", resp.StatusCode)
		return fmt.Errorf("API请求失败，状态码: %d", resp.StatusCode)
	}

	// 解析响应
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("读取响应失败: %v", err)
		return err
	}

	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		log.Printf("解析JSON失败: %v", err)
		return err
	}

	// 验证响应
	if apiResp.Status != 1 || len(apiResp.Data.Data) == 0 {
		log.Printf("API响应格式无效")
		return fmt.Errorf("API响应格式无效")
	}

	// 根据30分钟交易量排序
	tokens := apiResp.Data.Data
	sort.Slice(tokens, func(i, j int) bool {
		return tokens[i].Volume15m > tokens[j].Volume15m
	})

	// 仅保留前10个代币
	if len(tokens) > 10 {
		tokens = tokens[:10]
	}
	log.Printf("获取到30分钟内交易量最大的热门代币: %d个, 分别是 %+v", len(tokens), tokens)

	// 保存热门代币
	h.HotTokens = tokens

	// 为每个热门代币创建初始池信息结构
	for _, token := range h.HotTokens {
		info := TokenPoolsInfo{
			TokenAddress:   token.TargetToken,
			TokenSymbol:    token.TokenSymbol,
			Volume15m:      token.Volume15m,
			PumpPools:      []string{},
			MeteoraLists:   []string{},
			RaydiumPools:   []string{},
			RaydiumCPPools: []string{},
		}
		h.TokenPoolsInfos = append(h.TokenPoolsInfos, info)
		log.Printf("检测到15分钟内交易量大的代币: %s (%s), 15分钟交易量: $%.2f",
			token.TokenSymbol, token.TargetToken, token.Volume15m)
	}

	// 获取每个代币的池信息
	for i, info := range h.TokenPoolsInfos {
		h.FetchPoolsForToken(&h.TokenPoolsInfos[i], info.TokenAddress)
	}

	// 最后一次性更新配置
	h.UpdateConfig()

	return nil
}

// FetchPoolsForToken 获取指定代币的池信息
func (h *HotTokensTracker) FetchPoolsForToken(tokenInfo *TokenPoolsInfo, tokenAddress string) {
	// 构建Solscan API URL
	url := fmt.Sprintf("https://api-v2.solscan.io/v2/token/pools?page=1&page_size=40&token[]=%s", tokenAddress)

	// 创建请求
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("创建Solscan请求失败: %v", err)
		return
	}

	// 添加Solscan认证头
	req.Header.Set("x-sol-auth", h.AgentConfig.SolScan.SolAuth)
	req.Header.Set("authorization", h.AgentConfig.SolScan.Token)
	req.Header.Set("cookie", h.AgentConfig.SolScan.Cookie)
	req.Header.Set("origin", h.AgentConfig.SolScan.Origin)
	req.Header.Set("referer", h.AgentConfig.SolScan.Referer)
	req.Header.Set("Accept", "application/json")

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("请求Solscan API失败: %v", err)
		return
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != 200 {
		log.Printf("Solscan API请求失败，状态码: %d", resp.StatusCode)
		return
	}

	// 解析响应
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("读取Solscan响应失败: %v", err)
		return
	}

	var poolResp SolscanPoolResponse
	if err := json.Unmarshal(body, &poolResp); err != nil {
		log.Printf("解析Solscan JSON失败: %v", err)
		return
	}

	if !poolResp.Success || len(poolResp.Data) == 0 {
		log.Printf("Solscan没有返回有效数据")
		return
	}

	log.Printf("代币 %s (%s) 的池信息: %d个", tokenInfo.TokenSymbol, tokenInfo.TokenAddress, len(poolResp.Data))
	log.Printf("池信息: %+v", poolResp.Data)

	// 遍历data中的实际池，确保只处理与当前代币相关的池
	for _, pool := range poolResp.Data {
		poolID := pool.PoolID
		programID := pool.ProgramID

		// 获取池ID对应的账户信息
		poolAccount, hasPoolAccount := poolResp.Metadata.Accounts[poolID]
		if hasPoolAccount {
			// 检查是否为Pump池
			if strings.Contains(strings.ToLower(poolAccount.AccountLabel), "pump") &&
				!strings.Contains(strings.ToLower(poolAccount.AccountLabel), "bonding curve") {
				tokenInfo.PumpPools = append(tokenInfo.PumpPools, poolID)
				log.Printf("添加Pump池: %s, 标签: %v, 账户标签: %s",
					poolID, poolAccount.AccountTags, poolAccount.AccountLabel)
			}
		}

		// 获取程序ID对应的账户信息
		progAccount, hasProgAccount := poolResp.Metadata.Accounts[programID]
		if hasProgAccount {
			// 检查Raydium程序
			if strings.Contains(strings.ToLower(progAccount.AccountLabel), "raydium") &&
				progAccount.AccountType == "program" {
				// 区分普通池和集中流动性池
				if strings.Contains(strings.ToLower(progAccount.AccountLabel), "concentrated") ||
					strings.Contains(strings.ToLower(progAccount.AccountLabel), "clmm") {
					// 这是 Raydium CP 池
					tokenInfo.RaydiumCPPools = append(tokenInfo.RaydiumCPPools, poolID)
					log.Printf("添加 Raydium CP 池: %s", poolID)
				} else {
					// 这是普通 Raydium 池
					tokenInfo.RaydiumPools = append(tokenInfo.RaydiumPools, poolID)
					log.Printf("添加 Raydium 普通池: %s", poolID)
				}
			}

			// 检查Meteora程序
			if strings.Contains(strings.ToLower(progAccount.AccountLabel), "meteora") &&
				progAccount.AccountType == "program" {
				// 检查是否为 Meteora DLMM 池
				if strings.Contains(strings.ToLower(progAccount.AccountLabel), "dlmm") {
					tokenInfo.MeteoraLists = append(tokenInfo.MeteoraLists, poolID)
					log.Printf("添加 Meteora DLMM 池: %s, 标签: %s", poolID, progAccount.AccountLabel)
				}
			}
		}
	}

	log.Printf("代币 %s (%s) 的池信息: Pump池: %d, Meteora池: %d, Raydium池: %d, RaydiumCP池: %d",
		tokenInfo.TokenSymbol, tokenInfo.TokenAddress,
		len(tokenInfo.PumpPools), len(tokenInfo.MeteoraLists),
		len(tokenInfo.RaydiumPools), len(tokenInfo.RaydiumCPPools))
}

// UpdateConfig 根据搜集到的所有代币池信息更新配置
func (h *HotTokensTracker) UpdateConfig() {
	// 如果没有代币信息，不进行更新
	if len(h.TokenPoolsInfos) == 0 {
		log.Printf("没有代币信息可更新")
		return
	}

	// 根据交易量排序
	sort.Slice(h.TokenPoolsInfos, func(i, j int) bool {
		return h.TokenPoolsInfos[i].Volume15m > h.TokenPoolsInfos[j].Volume15m
	})

	// 只保留交易量最大且有至少两种类型池子的代币
	var validTokenInfos []TokenPoolsInfo
	for _, info := range h.TokenPoolsInfos {
		if !strings.Contains(info.TokenAddress, "pump") {
			continue // 过滤掉不含有Pump的代币，当前只交易pump
		}
		// 计算有多少种类型的池子
		poolTypeCount := 0
		if len(info.PumpPools) > 0 {
			poolTypeCount++
		}
		if len(info.MeteoraLists) > 0 {
			poolTypeCount++
		}
		if len(info.RaydiumPools) > 0 {
			poolTypeCount++
		}
		if len(info.RaydiumCPPools) > 0 {
			poolTypeCount++
		}

		// 只保留有至少两种类型池子的代币
		if poolTypeCount >= 2 {
			validTokenInfos = append(validTokenInfos, info)
			log.Printf("保留代币 %s (%s): 具有 %d 种类型的池子 (Pump: %d, Meteora: %d, Raydium: %d, RaydiumCP: %d)",
				info.TokenSymbol, info.TokenAddress, poolTypeCount,
				len(info.PumpPools), len(info.MeteoraLists),
				len(info.RaydiumPools), len(info.RaydiumCPPools))
		} else {
			log.Printf("过滤掉代币 %s (%s): 只有 %d 种类型的池子",
				info.TokenSymbol, info.TokenAddress, poolTypeCount)
		}
	}
	// 如果没有有效代币，不进行更新
	if len(validTokenInfos) == 0 {
		log.Printf("没有找到有池信息的代币")
		return
	}

	// 最多保留前两个
	if len(validTokenInfos) > 2 {
		validTokenInfos = validTokenInfos[:2]
	}

	// 创建MevConfig的副本
	newMevConfig := h.MevConfig.Copy()

	// 构建新的mint配置列表
	newMintConfigs := []MintConfig{}
	for _, info := range validTokenInfos {
		// 查找是否已存在该代币的配置
		var existingConfig *MintConfig
		for i := range newMevConfig.Routing.MintConfigList {
			if newMevConfig.Routing.MintConfigList[i].Mint == info.TokenAddress {
				existingConfig = &newMevConfig.Routing.MintConfigList[i]
				break
			}
		}

		// 构建新的配置或更新现有配置
		var mintConfig MintConfig
		if existingConfig != nil {
			// 复制现有配置
			mintConfig = *existingConfig
		} else {
			// 创建新配置
			mintConfig = MintConfig{
				Mint:                info.TokenAddress,
				LookupTableAccounts: []string{},
				ProcessDelay:        1000,
			}
		}

		// 更新池列表
		if len(info.PumpPools) > 0 {
			mintConfig.PumpPoolList = info.PumpPools
		}
		if len(info.MeteoraLists) > 0 {
			mintConfig.MeteoraPoolList = info.MeteoraLists
		}
		if len(info.RaydiumPools) > 0 {
			mintConfig.RaydiumPoolList = info.RaydiumPools
		}
		if len(info.RaydiumCPPools) > 0 {
			mintConfig.RaydiumCPPoolList = info.RaydiumCPPools
		}

		newMintConfigs = append(newMintConfigs, mintConfig)
	}

	// 更新配置
	newMevConfig.Routing.MintConfigList = newMintConfigs

	// 保存配置文件
	if err := newMevConfig.SaveToFile("config.toml"); err != nil {
		log.Printf("保存配置文件失败: %v", err)
		return
	}

	// 更新内存中的配置
	*h.MevConfig = *newMevConfig

	log.Printf("成功更新配置文件，添加/更新了%d个热门代币的池信息", len(newMintConfigs))
}

// StartTracking 启动跟踪协程
func (h *HotTokensTracker) StartTracking() {
	log.Println("启动热门代币跟踪器 - 按15分钟交易量排序")

	// 立即执行一次
	if err := h.FetchHotTokens(); err != nil {
		log.Printf("首次获取热门代币失败: %v", err)
	}
	h.Agent.RestartMEVBot()

	// 创建定时器
	ticker := time.NewTicker(h.PollInterval)
	for range ticker.C {
		if err := h.FetchHotTokens(); err != nil {
			log.Printf("获取热门代币失败: %v", err)
		}
		h.Agent.RestartMEVBot()
	}
}
