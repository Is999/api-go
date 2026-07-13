package keys

import "fmt"

// UserSessionHashKey 返回当前应用下的用户会话 Hash Key。
func UserSessionHashKey(userID int64) string {
	return WithPrefix(fmt.Sprintf(UserSessionHash, userID))
}

// UserSessionIndexKey 返回当前应用下的用户会话索引 Key。
func UserSessionIndexKey(userID int64) string {
	return WithPrefix(fmt.Sprintf(UserSessionIndex, userID))
}

// UserSessionAuthVersionKey 返回当前应用下的用户认证版本 Key。
func UserSessionAuthVersionKey(userID int64) string {
	return WithPrefix(fmt.Sprintf(UserSessionAuthVersion, userID))
}

// UserSessionKeys 返回会话 Lua 使用的同槽 Key，顺序为 Hash、索引、认证版本。
func UserSessionKeys(userID int64) []string {
	return []string{
		UserSessionHashKey(userID),
		UserSessionIndexKey(userID),
		UserSessionAuthVersionKey(userID),
	}
}
