package agent

import (
	"bytes"
	"encoding/json"
	"os"

	"github.com/pelletier/go-toml"
)

// Config 代表MEV Bot的整体配置
type Config struct {
	Routing struct {
		MintConfigList []MintConfig `toml:"mint_config_list"`
	} `toml:"routing"`
	RPC struct {
		URL string `toml:"url"`
	} `toml:"rpc"`
	Spam struct {
		Enabled          bool     `toml:"enabled"`
		SendingRPCURLs   []string `toml:"sending_rpc_urls"`
		ComputeUnitPrice struct {
			Strategy string `toml:"strategy"`
			From     int    `toml:"from"`
			To       int    `toml:"to"`
			Count    int    `toml:"count"`
		} `toml:"compute_unit_price"`
		MaxRetries       int  `toml:"max_retries"`
		EnableSimpleSend bool `toml:"enable_simple_send"`
	} `toml:"spam"`
	Jito struct {
		Enabled         bool     `toml:"enabled"`
		BlockEngineURLs []string `toml:"block_engine_urls"`
		UUID            string   `toml:"uuid"`
		IPAddresses     []string `toml:"ip_addresses"`
		TipConfig       struct {
			Strategy string `toml:"strategy"`
			From     int    `toml:"from"`
			To       int    `toml:"to"`
			Count    int    `toml:"count"`
		} `toml:"tip_config"`
	} `toml:"jito"`
	KaminoFlashloan struct {
		Enabled bool `toml:"enabled"`
	} `toml:"kamino_flashloan"`
	Bot struct {
		ComputeUnitLimit int  `toml:"compute_unit_limit"`
		MergeMints       bool `toml:"merge_mints"`
	} `toml:"bot"`
	Wallet struct {
	} `toml:"wallet"`
}

// MintConfig 代表代币铸币配置
type MintConfig struct {
	Mint                string   `toml:"mint" json:"mint"`
	PumpPoolList        []string `toml:"pump_pool_list" json:"pump_pool_list"`
	RaydiumPoolList     []string `toml:"raydium_pool_list" json:"raydium_pool_list"`
	RaydiumCPPoolList   []string `toml:"raydium_cp_pool_list" json:"raydium_cp_pool_list"`
	MeteoraPoolList     []string `toml:"meteora_dlmm_pool_list" json:"meteora_dlmm_pool_list"`
	LookupTableAccounts []string `toml:"lookup_table_accounts" json:"lookup_table_accounts"`
	ProcessDelay        int      `toml:"process_delay" json:"process_delay"`
}

// LoadConfig 从文件加载配置
func LoadConfig(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// SaveToFile 将配置保存到文件
func (c *Config) SaveToFile(configPath string) error {
	// 将配置编码为TOML
	data, err := toml.Marshal(c)
	if err != nil {
		return err
	}

	// 写入文件
	return os.WriteFile(configPath, data, 0644)
}

// Copy 创建配置的深度副本
func (c *Config) Copy() *Config {
	data, _ := json.Marshal(c)
	var copy Config
	json.Unmarshal(data, &copy)
	return &copy
}

// UpdateSection 更新特定节的配置
func (c *Config) UpdateSection(section, key string, value interface{}) error {
	// 将配置转换为TOML树
	configTree, err := toml.Marshal(c)
	if err != nil {
		return err
	}

	var tree toml.Tree
	err = toml.Unmarshal(configTree, &tree)
	if err != nil {
		return err
	}

	// 获取要更新的节
	sectionPath := section
	if key != "" {
		sectionPath = section + "." + key
	}

	// 更新值
	tree.Set(sectionPath, value)

	// 将更新后的树重新解析为配置
	var buf bytes.Buffer
	toml.NewEncoder(&buf).Encode(tree)

	return toml.Unmarshal(buf.Bytes(), c)
}
