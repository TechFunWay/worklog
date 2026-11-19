// migration.go — v0.6.0 三项粉丝需求（记工补贴、借支选工地、结算分期实付）的数据回填。
//
// 结构（加列/加表）由 AutoMigrate 负责；这里只做「老数据补值」，全部幂等：
// 每条 UPDATE 都带一个跑过就不会再命中的哨兵条件，重复执行无副作用。
package worklog

import (
	"gorm.io/gorm"
	"smallgo/server/database"
)

// 注意：Version 必须 ≤ 运行中二进制的 version.Version，否则 RunUpgrades 会跳过本脚本。
// 发版时 VERSION 同步改成 0.6.0（三处：VERSION / app.json / fnpack/manifest）。
// 万一脚本没跑，fillSettlementDerived 与工资条展示也有兜底，不会把历史工资条
// 显示成「全额欠薪」，但 wage_cents 会缺值（明细里工钱列显示 0），仍需正常升级。
func init() {
	database.Upgrades = append(database.Upgrades, database.Upgrade{
		Version: "0.6.0",
		Name:    "worklog_subsidy_advance_project_settlement_paid",
		Upgrade: func(db *gorm.DB) error {
			// 1) 记工：老记录的 amount 全部是纯工钱，补贴/扣款为 0。
			//    哨兵 wage_cents = 0 且 amount_cents <> 0，跑过的行不会再命中。
			if err := db.Exec(`UPDATE work_records SET wage_cents = amount_cents WHERE wage_cents = 0 AND amount_cents <> 0`).Error; err != nil {
				return err
			}
			// 2) 结算单：v0.5.x 的结算语义就是「一次结清并付清」，回填成已付清；
			//    否则新加的已付/尚欠会把整批历史工资条显示成全额欠薪。
			//    哨兵 paid_cents = 0：新机制产生的未付单 pay_status 是 unpaid/partial，不会被误改。
			if err := db.Exec(
				"UPDATE settlements SET paid_cents = payable_cents WHERE paid_cents = 0 AND pay_status = ?", PayStatusPaid).Error; err != nil {
				return err
			}
			// 3) 借支的历史行 project_id 保持 NULL（未指定工地），行为与升级前一致：
			//    只在按人结算时抵扣，不参与任何按项目结算 —— 无需回填，也不许回填成某个工地。
			return nil
		},
	})
}
