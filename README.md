
#  Agentic Defense Matrix (ADM)

> **The Unified Blue/Green Team Architecture for Agentic AI Systems**

一個針對具備自主規劃與工具調用（Tool-calling）能力的 Agentic AI 所設計的縱深防禦系統。本專案屏棄傳統僅依賴「提示詞過濾」的無效防護，透過作業系統底層遙測（Telemetry）、動態權限管控與狀態感知 SIEM，徹底限制 AI 代理的爆炸半徑。

---

##  目標 (Objective)

建立一套涵蓋 L7 API 閘道層至 OS 端點層的全方位防禦矩陣。確保在 Agent 遭遇**間接提示詞注入 (Data Poisoning)**、**混淆代理人攻擊 (Confused Deputy)** 或**狀態漂移 (State Drift)** 時，系統能主動識別語意變異，並於底層阻斷未經授權的系統呼叫與資料外洩，確保宿主環境的絕對安全。

##  手段 (Methods)

本專案採用「藍隊偵測 + 綠隊隔離」的聯合防禦機制：

1. **跨維度遙測 (Cross-dimensional Telemetry)：** 結合 API 閘道的語意向量分析與 OS 底層（WFP / macOS Endpoint Security）的程序/網路攔截。
2. **狀態感知 SIEM (Stateful SIEM)：** 將自然語言的對話意圖與底層系統呼叫（Syscalls）進行時間序列的關聯性比對。
3. **零信任架構與微隔離 (Micro-segmentation)：** 實作動態 IAM 權限降級，並將 Agent 的工具執行環境推入拋棄式沙盒 (Ephemeral Sandboxing)。

##  限制 (Constraints & Limitations)

* **效能開銷：** 底層的網路封包攔截與 SIEM 關聯分析必須滿足極低延遲，不可對正常 Agent 推論造成超過 50ms 的阻礙。
* **無狀態假設：** Agent 本身必須設計為無狀態（Stateless），所有的狀態與記憶由外部受控的資料庫管理，以便綠隊隨時銷毀並重啟 Agent 容器。
* **封閉性網路：** 執行環境必須實施嚴格的出境過濾（Egress Filtering），除白名單 API 外，預設丟棄所有對外連線。

---

##  技術棧 (Tech Stack)

* **API Gateway & SIEM Engine:** `Go` (基於 Echo / Gin 框架，提供高併發的語意攔截與日誌接收)。
* **Endpoint Telemetry Daemon (Watchdog / Gate_God):** `Rust` (利用 WFP API 與 macOS Endpoint Security 實作記憶體安全的底層攔截器)。
* **Sandboxing:** `Docker` API / `WebAssembly` (Wasmtime)。
* **Authentication:** 動態 STS Token (AWS IAM / HashiCorp Vault)。

---

##  專案結構 (Repository Structure)

```text
agentic-defense-matrix/
├── .github/
│   └── workflows/
│       ├── ci.yml                 # Go & Rust 單元測試與建置
│       └── red_team_fuzz.yml      # CI/CD 階段的自動化 PyRIT 紅隊越獄測試
├── cmd/
│   ├── gateway/                 # Go: API 閘道器與語意變異分析引擎
│   └── siem_engine/             # Go: 狀態感知關聯分析伺服器
├── pkg/
│   ├── auth/                    # Go: 動態 IAM 與 Token 撤銷邏輯
│   ├── semantic/                # Go: 提示詞向量化與惡意意圖比對
│   └── telemetry/               # Go: 日誌格式化與傳輸介面
├── agents/                      # 微型 Agent 實作 (Planner, Executor, Summarizer)
│   └── executor/
│       └── sandbox/             # Docker/Wasm 拋棄式環境設定檔
├── daemon_watchdog/             # Rust: 部署於端點的藍隊監控代理 (Gate_God核心)
│   ├── Cargo.toml
│   ├── src/
│   │   ├── main.rs
│   │   ├── wfp_filter.rs        # Windows Filtering Platform 封包攔截實作
│   │   ├── macos_es.rs          # macOS Endpoint Security 程序監控實作
│   │   └── egress_blocker.rs    # 綠隊: 動態出境網路阻擋器
├── docs/                        # 架構圖與 MITRE ATLAS 威脅建模文件
├── tests/
│   └── integration/             # 紅藍隊攻防整合測試腳本
└── README.md

```

---

##  實施方法 (Implementation Methods)

1. **L7 語意防禦 (Gateway Layer):** 透過 Go 撰寫的 Middleware 攔截所有進入 Agent 的請求，計算短時間內的語意相似度，阻擋自動化高頻紅隊工具。
2. **OS 行為遏制 (Endpoint Layer):** 部署 Rust 撰寫的 `watchdog` 守護行程，掛載 WFP 過濾器，將 Agent 產生的 Socket 連線與 API Session ID 強制綁定。
3. **綠隊動態修復 (Green Team Response):** 當 SIEM 判定威脅，觸發 Webhook，透過 API 撤銷該 Session 的 IAM 權限，並發送 SIGKILL 終止該 Agent 所在的沙盒容器。

##  實施計畫 (Implementation Plan)

* **Phase 1: 架構重構與沙盒化 (Week 1-2)**
* 分離 Agent 的對話與執行模組，建置 Docker/Wasm 拋棄式執行環境。


* **Phase 2: 底層攔截器部署 (Week 3-4)**
* 完成 Rust 端點程式的開發，實作 WFP 網路封包攔截並匯出關聯性日誌。


* **Phase 3: SIEM 關聯引擎建置 (Week 5-6)**
* 使用 Go (Echo/Gin) 建立日誌接收端，撰寫基於 MITRE ATLAS 的時間序列偵測規則。


* **Phase 4: 動態權限與自動化阻擋 (Week 7-8)**
* 整合 IAM 系統，實作威脅觸發時的 Token 撤銷與網路出境封鎖 (Egress Drop)。



---

##  驗收方法與計畫 (Acceptance Criteria & Plan)

驗收將採用「自動化 Attacker Agent 靶場對抗」的形式進行。

| 測試階段 | 攻擊情境 (Red Team) | 預期防禦行為 (Blue/Green Team) | 驗收標準 |
| --- | --- | --- | --- |
| **Stage 1: API 邊界** | 遠端 Agent 發動高頻、語意相似的間接提示詞注入。 | Go Gateway 偵測到語意變異度異常。 | 觸發 Rate Limit，回傳隨機混淆錯誤代碼，成功阻擋 95% 探測。 |
| **Stage 2: 邏輯濫用** | 混淆代理人攻擊：誘騙 Agent 依序調用讀取機密工具與對外發信工具。 | Rust `watchdog` 捕捉到異常程序的系統呼叫鏈結，SIEM 觸發規則。 | IAM 瞬間降權，外寄動作被沙盒網路層拒絕。 |
| **Stage 3: 系統滲透** | 利用 RAG 知識庫投毒，誘使 Agent 在本地執行 Reverse Shell 指令。 | macOS ES / WFP 攔截到未經授權的子程序生成 (e.g., `bash -i`)。 | 程序建立失敗，沙盒容器被立刻強制銷毀。 |

---

##  學術與資料來源 (References & Academic Sources)

1. **威脅建模體系：** [MITRE ATLAS (Adversarial Threat Landscape for AI Systems)](https://atlas.mitre.org/) - 核心戰術參考 (AML.T0015, AML.T0020)。
2. **架構安全規範：** [OWASP Top 10 for Large Language Model Applications](https://owasp.org/www-project-top-10-for-large-language-model-applications/) - 針對 LLM01 (Prompt Injection) 與 LLM08 (Excessive Agency) 的緩解實務。
3. **風險分析框架：** [Berryville Institute of Machine Learning (BIML) - Architectural Risk Analysis](https://berryvilleiml.com/) - 關於資料與指令邊界模糊的底層設計原則。
4. **雲端零信任實踐：** [Cloud Security Alliance (CSA) AI Safety Guidelines](https://cloudsecurityalliance.org/) - 動態 IAM 與微服務隔離策略。
5. **前沿學術理論：** Harvard University `CS 2881: AI Safety` (Boaz Barak) - 關於模型行為邊界 (Model Specs)、紅藍隊對抗與越獄機制的理論基礎。
