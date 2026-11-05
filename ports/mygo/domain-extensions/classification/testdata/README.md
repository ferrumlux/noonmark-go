# 合成旧行为记录

legacy-equivalent-rename.json 由未修复的正式 Go engine（a91eba40）在隔离外置探针中真实生成：建立É，再重命名为E+U+0301。含固定UUID与合成时间，无用户资料。版本unicode-fixture-nfkc-casefold-whitespace-v1明确是测试名称表，不是完整规范器。

该记录保存旧错误的revision2及两份nameVersions。修复后的严格重放应拒绝，不能静默“修正”输入。此文件不是Swift导出、runtime v2会话或生产迁移fixture。
