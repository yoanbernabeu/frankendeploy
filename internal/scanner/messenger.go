package scanner

import (
	"regexp"
	"sort"
	"strings"
)

// envPlaceholderRegex matches Symfony env placeholders like
// '%env(MESSENGER_TRANSPORT_DSN)%' or '%env(resolve:MESSENGER_TRANSPORT_DSN)%'.
var envPlaceholderRegex = regexp.MustCompile(`%env\(([A-Za-z0-9_:]+)\)%`)

// messengerTransportInfo describes the async transports a worker must consume.
type messengerTransportInfo struct {
	// Transports to consume, sorted for deterministic output
	Transports []string
	UsesAMQP   bool
	UsesRedis  bool
}

// detectMessengerTransports reads messenger.yaml and resolves each transport
// DSN (through .env/.env.local for %env()% placeholders). The failure
// transport and sync/in-memory transports are excluded: consuming them is
// either wrong or impossible, and a wrong transport name means a
// crash-looping worker under --restart unless-stopped.
func (s *Scanner) detectMessengerTransports(env map[string]string) messengerTransportInfo {
	info := messengerTransportInfo{}

	mc, err := s.GetMessengerConfig()
	if err != nil {
		return info
	}

	failure := mc.Framework.Messenger.FailureTransport

	names := make([]string, 0, len(mc.Framework.Messenger.Transports))
	for name := range mc.Framework.Messenger.Transports {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if name == failure {
			continue
		}
		dsn := resolveTransportDSN(mc.Framework.Messenger.Transports[name], env)
		if strings.HasPrefix(dsn, "sync://") || strings.HasPrefix(dsn, "in-memory://") {
			continue
		}
		info.Transports = append(info.Transports, name)
		switch {
		case strings.HasPrefix(dsn, "amqp://"), strings.HasPrefix(dsn, "amqps://"):
			info.UsesAMQP = true
		case strings.HasPrefix(dsn, "redis://"), strings.HasPrefix(dsn, "rediss://"):
			info.UsesRedis = true
		}
	}

	return info
}

// resolveTransportDSN extracts the DSN from a transport definition (plain
// string or map with a dsn key) and resolves %env()% placeholders against
// the merged project env.
func resolveTransportDSN(transport interface{}, env map[string]string) string {
	var dsn string
	switch v := transport.(type) {
	case string:
		dsn = v
	case map[string]interface{}:
		if d, ok := v["dsn"].(string); ok {
			dsn = d
		}
	}

	if m := envPlaceholderRegex.FindStringSubmatch(dsn); len(m) > 1 {
		key := m[1]
		// Strip env processors (resolve:, default::, ...) — the variable
		// name is the last segment
		if idx := strings.LastIndex(key, ":"); idx != -1 {
			key = key[idx+1:]
		}
		if val, ok := env[key]; ok {
			dsn = val
		}
	}

	return dsn
}
