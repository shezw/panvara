<!--
    Panvara
    docs/adr/0001-module-data-revision-identities.md    2026-07-15
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# ADR-0001：拆分模块、数据结构与 Source 身份

- 状态：已接受
- 日期：2026-07-15
- 适用范围：alpha.3a 开发切片

## 背景

alpha.2 使用完整 Canonical IR 的 Revision Hash 同时标识编译结果和 Record 数据空间。标签、Manager 展示或 API 白名单等非数据结构变化也会改变该 Hash，因此不能仅凭它判断两份模型的数据结构是否相同。

后续发布、迁移和回滚需要先回答“模块内容是否相同”“数据结构是否相同”“作者提交的原始字节是否相同”三个不同问题。若继续混用一个版本号，迁移计划和审计会产生歧义。

## 决策

Panvara 明确维护四类身份；其中数据结构身份是一个可追加集合，而不是父 Revision 上的一对固定字段：

| 身份 | 产生方式 | 回答的问题 | alpha.3a 用途 |
| --- | --- | --- | --- |
| Module Version | 作者填写的 SemVer | 作者怎样标记业务版本 | 展示与审计元数据 |
| Module Revision | 完整 Canonical IR 的 SHA-256 | 完整编译语义是否相同 | 编译制品身份；继续作为当前 Record namespace |
| Data Schema Identity | `{format, fingerprint}`；指纹是该格式投影的 SHA-256 | 在指定投影算法下，持久化数据结构是否相同 | 只读比较和未来迁移规划输入 |
| Source Hash | 原始 Source 字节的 SHA-256 | 作者输入字节是否相同 | 来源证明、下载 ETag 与篡改检查 |

一个不可变 Module Revision 是父制品，保存 Source、Canonical IR、OpenAPI、Manager UI Schema 和首次登记 provenance。它拥有按 `format` 升序返回的 `data_schema_identities` 子身份数组；每项精确包含：

```json
{"format": 1, "fingerprint": "sha256:<64 lowercase hex>"}
```

父 Revision Hash 只由完整 Canonical IR 决定，不包含这组派生身份。未来新增投影算法时，可以给已有父 Revision 追加新的格式身份，而不改写父 Revision、Source、首次登记信息或已有格式身份。同一 `Revision + format` 只能对应一个 fingerprint；出现冲突表示数据损坏，必须 fail closed。

Data Schema Projection format 1 包含：

- 模块名称；
- Resource 名称；
- Field 名称、类型、required、unique；
- reference 目标、enum 候选值和字段 constraints。

投影对 Resource、Field 和 enum 候选值进行确定性排序。它不包含 Module SemVer、labels、Manager、API、Capability、模块依赖或冲突。格式号与指纹必须一起保存；未来改变投影规则时新增格式号，不在原格式上重解释或覆盖旧指纹。消费者必须按 `format` 查找，不能依赖数组位置。

## 约束

- Data Schema Identity 不是发布状态、激活指针或 Record namespace。
- alpha.3a 不改变 alpha.2 的 Record Scope；运行时仍使用完整 Module Revision。
- 相同 format 下的相同 fingerprint 只能说明该格式投影相同，不能证明完整模块、Source 或迁移策略相同。
- `data_schema_identities` 必须按 format 升序，format 为正整数且不能重复；当前编译器至少提供 format 1。
- 已有 Data Schema Identity 以及父 Revision 都禁止 UPDATE、DELETE 或重新解释；新增算法只能追加新的 format。
- 当前活动 Revision 必须从运行中 OpenAPI 的 `x-panvara-revision` 读取，不能把 Registry List 的第一项当作活动版本。

## 结果

纯标签、Manager、API、Capability、依赖和 SemVer 变化会产生新的 Module Revision，但两个父 Revision 的 format 1 fingerprint 可以相同。字段、引用、enum 或约束变化会改变 format 1 fingerprint。

这一拆分为未来 Plan/Publish/Activate 提供可验证输入，同时允许投影算法演进而不改变父 Revision 身份。迁移兼容性仍需独立规则判断，不能由“指纹相同/不同”直接推出。

## 未决定事项

- 哪些 Data Schema Identity format 参与迁移计划和激活前检查；
- Record namespace 是否在未来从 Module Revision 迁移到独立 Data Schema Revision；
- 跨投影格式的兼容矩阵与升级协议。

这些事项必须由后续 ADR 决定，alpha.3a 不预埋隐式切换。
