// Package render turns a raw Avi API JSON result into a structured HTML
// table for the object types common enough to deserve fixed columns
// (virtual services, pools, health monitors, service engines), falling back
// to nothing recognized when the shape doesn't match -- the caller keeps the
// original fenced-JSON block as the fallback in that case.
package render

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"
)

const dash = "—"

// ObjectTable renders a table for jsonText if it recognizes the shape of a
// known Avi object type, either a list envelope ({"count":N,"results":[...]})
// or a single object. Returns "" when the JSON doesn't parse or matches
// nothing known, which the caller treats as "no table, keep the raw JSON."
func ObjectTable(jsonText string) string {
	var raw interface{}
	if err := json.Unmarshal([]byte(jsonText), &raw); err != nil {
		return ""
	}

	var rows []map[string]interface{}
	switch v := raw.(type) {
	case map[string]interface{}:
		if results, ok := v["results"].([]interface{}); ok {
			for _, r := range results {
				if rm, ok := r.(map[string]interface{}); ok {
					rows = append(rows, rm)
				}
			}
		} else {
			rows = append(rows, v)
		}
	default:
		return ""
	}
	if len(rows) == 0 {
		return ""
	}

	kind := classify(rows[0])
	if kind == "" {
		return ""
	}
	columns, rowFn := columnsFor(kind)
	if columns == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString(`<table class="object-table"><thead><tr>`)
	for _, c := range columns {
		b.WriteString("<th>")
		b.WriteString(html.EscapeString(c))
		b.WriteString("</th>")
	}
	b.WriteString(`</tr></thead><tbody>`)
	for _, row := range rows {
		// A mixed-shape list (shouldn't happen for a single Avi list call,
		// but a hand-rolled execute_generic_operation could produce one) is
		// skipped row-by-row rather than mis-rendered under the wrong columns.
		if classify(row) != kind {
			continue
		}
		b.WriteString("<tr>")
		for _, cell := range rowFn(row) {
			b.WriteString("<td>")
			b.WriteString(html.EscapeString(cell))
			b.WriteString("</td>")
		}
		b.WriteString("</tr>")
	}
	b.WriteString(`</tbody></table>`)

	// A single pool's member list is worth its own nested table -- pool
	// members aren't a listable Avi object type on their own, so this is the
	// only place they can be shown structured.
	if kind == "pool" && len(rows) == 1 {
		b.WriteString(poolMembersTable(rows[0]))
	}

	return b.String()
}

// classify guesses an Avi object type from field presence -- the MCP tool
// names (avi_list/avi_get) don't encode the object type themselves (it's an
// argument, not part of the name), so content is the only signal available
// by the time renderChatMessage sees the result.
func classify(m map[string]interface{}) string {
	switch {
	case has(m, "vsvip_ref"), has(m, "services") && has(m, "cloud_ref"):
		return "virtualservice"
	case has(m, "lb_algorithm"), has(m, "servers"):
		return "pool"
	case has(m, "send_interval") && has(m, "receive_timeout"):
		return "healthmonitor"
	case has(m, "mgmt_ip_address"):
		return "serviceengine"
	default:
		return ""
	}
}

func columnsFor(kind string) ([]string, func(map[string]interface{}) []string) {
	switch kind {
	case "virtualservice":
		return []string{"VIRTUAL SERVICE", "VIP", "HEALTH", "CONNS", "POOL"}, virtualServiceRow
	case "pool":
		return []string{"POOL", "HEALTH", "MEMBERS", "LB ALGORITHM", "PORT"}, poolRow
	case "healthmonitor":
		return []string{"HEALTH MONITOR", "TYPE", "PORT", "INTERVAL", "TIMEOUT"}, healthMonitorRow
	case "serviceengine":
		return []string{"SERVICE ENGINE", "STATUS", "MGMT IP", "VS COUNT"}, serviceEngineRow
	default:
		return nil, nil
	}
}

// virtualServiceRow's CONNS column is always "—": live connection counts
// live on the analytics/runtime endpoints, not the virtualservice config
// object this table is built from.
func virtualServiceRow(m map[string]interface{}) []string {
	vip := refName(m, "vsvip_ref")
	if vip == "" {
		vip = dash
	}
	pool := refName(m, "pool_ref")
	if pool == "" {
		pool = refName(m, "pool_group_ref")
	}
	if pool == "" {
		pool = dash
	}
	return []string{orDash(str(m, "name")), vip, enabledLabel(m), dash, pool}
}

func poolRow(m map[string]interface{}) []string {
	members := dash
	if servers, ok := m["servers"].([]interface{}); ok {
		up := 0
		for _, sv := range servers {
			if sm, ok := sv.(map[string]interface{}); ok {
				if enabled, ok := sm["enabled"].(bool); ok && enabled {
					up++
				}
			}
		}
		members = fmt.Sprintf("%d/%d up", up, len(servers))
	}
	port := dash
	if p, ok := m["default_server_port"]; ok {
		port = fmt.Sprint(p)
	}
	return []string{orDash(str(m, "name")), enabledLabel(m), members, orDash(str(m, "lb_algorithm")), port}
}

func poolMembersTable(pool map[string]interface{}) string {
	servers, ok := pool["servers"].([]interface{})
	if !ok || len(servers) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(`<table class="object-table object-table-nested"><thead><tr>`)
	b.WriteString(`<th>SERVER</th><th>PORT</th><th>RATIO</th><th>STATUS</th></tr></thead><tbody>`)
	for _, sv := range servers {
		sm, ok := sv.(map[string]interface{})
		if !ok {
			continue
		}
		ip := dash
		if ipObj, ok := sm["ip"].(map[string]interface{}); ok {
			if addr, ok := ipObj["addr"].(string); ok {
				ip = addr
			}
		}
		port := dash
		if p, ok := sm["port"]; ok {
			port = fmt.Sprint(p)
		}
		ratio := dash
		if r, ok := sm["ratio"]; ok {
			ratio = fmt.Sprint(r)
		}
		b.WriteString("<tr><td>")
		b.WriteString(html.EscapeString(ip))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(port))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(ratio))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(enabledLabel(sm)))
		b.WriteString("</td></tr>")
	}
	b.WriteString(`</tbody></table>`)
	return b.String()
}

func healthMonitorRow(m map[string]interface{}) []string {
	port := dash
	if p, ok := m["monitor_port"]; ok {
		port = fmt.Sprint(p)
	}
	interval := dash
	if v, ok := m["send_interval"]; ok {
		interval = fmt.Sprint(v) + "s"
	}
	timeout := dash
	if v, ok := m["receive_timeout"]; ok {
		timeout = fmt.Sprint(v) + "s"
	}
	return []string{orDash(str(m, "name")), orDash(str(m, "type")), port, interval, timeout}
}

func serviceEngineRow(m map[string]interface{}) []string {
	mgmtIP := dash
	if ip, ok := m["mgmt_ip_address"].(map[string]interface{}); ok {
		if addr, ok := ip["addr"].(string); ok {
			mgmtIP = addr
		}
	}
	vsCount := dash
	if refs, ok := m["vs_refs"].([]interface{}); ok {
		vsCount = fmt.Sprint(len(refs))
	}
	return []string{orDash(str(m, "name")), enabledLabel(m), mgmtIP, vsCount}
}

func enabledLabel(m map[string]interface{}) string {
	if enabled, ok := m["enabled"].(bool); ok {
		if enabled {
			return "Enabled"
		}
		return "Disabled"
	}
	return dash
}

func has(m map[string]interface{}, key string) bool {
	_, ok := m[key]
	return ok
}

func str(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func orDash(s string) string {
	if s == "" {
		return dash
	}
	return s
}

// refName pulls the human-readable name Avi appends after "#" on a ref URL
// when the caller queried with include_name (e.g. ".../pool/pool-<uuid>#my-pool").
// Without that query param a ref is just a UUID, not worth showing in a
// table meant to be scannable -- callers treat "" as "show a dash instead."
func refName(m map[string]interface{}, key string) string {
	ref := str(m, key)
	if idx := strings.LastIndex(ref, "#"); idx != -1 && idx+1 < len(ref) {
		return ref[idx+1:]
	}
	return ""
}
