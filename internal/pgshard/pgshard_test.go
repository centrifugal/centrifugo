package pgshard

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Values computed by PostgreSQL 16 (UTF8): SELECT hashtext(s),
// abs(hashtext(s)::bigint) % 16, abs(hashtext(s)::bigint) % 7. The keys cover
// every remainder length of the 12-byte blocks, several blocks, and
// multi-byte characters.
var postgresHashtext = []struct {
	key      string
	hashtext int32
	shard16  int
	shard7   int
}{
	{"", -1477818771, 3, 2},
	{"a", 1075015857, 1, 6},
	{"ab", 1718550461, 13, 5},
	{"abc", -785388649, 9, 3},
	{"abcd", -393934804, 4, 4},
	{"abcde", -445659580, 12, 2},
	{"abcdef", -1747460160, 0, 5},
	{"abcdefg", 501636814, 14, 0},
	{"abcdefgh", -1960928205, 13, 5},
	{"abcdefghi", -92131489, 1, 2},
	{"abcdefghij", 1948051852, 12, 5},
	{"abcdefghijk", -1483803693, 13, 1},
	{"abcdefghijkl", -1586087212, 12, 3},
	{"abcdefghijklm", 405849808, 0, 0},
	{"abcdefghijklmn", 315215413, 5, 2},
	{"abcdefghijklmno", -1389283689, 9, 3},
	{"abcdefghijklmnop", -2079749669, 5, 4},
	{"abcdefghijklmnopq", 1648114242, 2, 5},
	{"abcdefghijklmnopqr", -952297252, 4, 4},
	{"abcdefghijklmnopqrs", -684650393, 9, 0},
	{"abcdefghijklmnopqrst", 766334193, 1, 2},
	{"abcdefghijklmnopqrstu", 2124303250, 2, 6},
	{"abcdefghijklmnopqrstuv", -50608962, 2, 5},
	{"abcdefghijklmnopqrstuvw", -1149376136, 8, 6},
	{"abcdefghijklmnopqrstuvwx", 1251586959, 15, 0},
	{"abcdefghijklmnopqrstuvwxy", 980150893, 13, 1},
	{"abcdefghijklmnopqrstuvwxyz", 167831483, 11, 1},
	{"abcdefghijklmnopqrstuvwxyz0", -1923549396, 4, 6},
	{"abcdefghijklmnopqrstuvwxyz01", -636332337, 1, 4},
	{"abcdefghijklmnopqrstuvwxyz012", -412030706, 2, 3},
	{"abcdefghijklmnopqrstuvwxyz0123", 89653194, 10, 1},
	{"abcdefghijklmnopqrstuvwxyz01234", -908372798, 14, 4},
	{"abcdefghijklmnopqrstuvwxyz012345", 1203288424, 8, 2},
	{"abcdefghijklmnopqrstuvwxyz0123456", -19649275, 11, 2},
	{"abcdefghijklmnopqrstuvwxyz01234567", -189095804, 12, 2},
	{"abcdefghijklmnopqrstuvwxyz012345678", -1475196007, 7, 5},
	{"abcdefghijklmnopqrstuvwxyz0123456789", 1318422268, 12, 2},
	{"abcdefghijklmnopqrstuvwxyz0123456789A", -22154523, 11, 6},
	{"abcdefghijklmnopqrstuvwxyz0123456789AB", 721360856, 8, 6},
	{"abcdefghijklmnopqrstuvwxyz0123456789ABC", -235185553, 1, 1},
	{"abcdefghijklmnopqrstuvwxyz0123456789ABCD", 1451736991, 15, 5},
	{"chat:room", -114479489, 1, 5},
	{"привет мир", 1629092517, 5, 3},
	{"/tenant/users/42", -785199677, 13, 3},
	{"emoji:🎉", 194887847, 7, 0},
	{"test:hold_by_node", 71035620, 4, 5},
	{"$personal:user#42", 1513160943, 15, 0},
}

func TestOfMatchesPostgres(t *testing.T) {
	for _, tc := range postgresHashtext {
		require.Equal(t, tc.hashtext, int32(hashBytes([]byte(tc.key))), "hashtext(%q)", tc.key)
		require.Equal(t, tc.shard16, Of(tc.key, 16), "shard of %q among 16", tc.key)
		require.Equal(t, tc.shard7, Of(tc.key, 7), "shard of %q among 7", tc.key)
	}
}
