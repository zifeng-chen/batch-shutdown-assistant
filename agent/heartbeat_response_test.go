package main

import (
	"encoding/json"
	"testing"
)

// 与 sendHeartbeat 中的解析结构保持一致，用于回归服务端响应形状。
func decodeHeartbeatResponse(t *testing.T, body string) (deviceID, token, action string, hours *int) {
	t.Helper()
	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
		Msg  string          `json:"msg"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("解信封失败: %v", err)
	}
	var result struct {
		DeviceID     interface{} `json:"device_id"`
		Token        string      `json:"token"`
		Action       string      `json:"action"`
		OfflineHours *int        `json:"offline_selfdestruct_hours"`
	}
	if len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		if err := json.Unmarshal(envelope.Data, &result); err != nil {
			t.Fatalf("解析 data 失败: %v", err)
		}
	}
	return normalizeDeviceID(result.DeviceID), result.Token, result.Action, result.OfflineHours
}

// 服务端四种响应动作都必须能取出 device_id。
func TestHeartbeatEnvelopeAllActions(t *testing.T) {
	cases := []struct{ name, body, wantID, wantAction, wantToken string }{
		{"created", `{"code":0,"data":{"action":"created","device_id":26,"token":"tok","commands":[]},"msg":"success"}`, "26", "created", "tok"},
		{"ok", `{"code":0,"data":{"action":"ok","device_id":27,"token":"tok","commands":[],"offline_selfdestruct_hours":0},"msg":"success"}`, "27", "ok", "tok"},
		{"re-enrolled", `{"code":0,"data":{"action":"re-enrolled","device_id":28,"token":"tok","commands":[]},"msg":"success"}`, "28", "re-enrolled", "tok"},
		// deleted 分支服务端不带 token
		{"deleted", `{"code":0,"data":{"action":"deleted","device_id":29},"msg":"success"}`, "29", "deleted", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id, tok, act, _ := decodeHeartbeatResponse(t, c.body)
			if id != c.wantID || act != c.wantAction || tok != c.wantToken {
				t.Fatalf("id=%q action=%q token=%q", id, act, tok)
			}
		})
	}
}

// 迁移前的 UUID 主键也要能接住。
func TestHeartbeatLegacyUUID(t *testing.T) {
	body := `{"code":0,"data":{"action":"ok","device_id":"6f1c2b1e-0f0a-4a5e-9a3d-1e2f3a4b5c6d","token":"tok"},"msg":"success"}`
	if id, _, _, _ := decodeHeartbeatResponse(t, body); id != "6f1c2b1e-0f0a-4a5e-9a3d-1e2f3a4b5c6d" {
		t.Fatalf("legacy uuid = %q", id)
	}
}

// 阈值缺失时不能把现值误清成 0。
func TestHeartbeatOfflineHoursAbsent(t *testing.T) {
	body := `{"code":0,"data":{"action":"created","device_id":30,"token":"tok"},"msg":"success"}`
	if _, _, _, hours := decodeHeartbeatResponse(t, body); hours != nil {
		t.Fatalf("缺失时不应有值，得到 %d", *hours)
	}
}

func TestHeartbeatOfflineHoursPresent(t *testing.T) {
	body := `{"code":0,"data":{"action":"ok","device_id":1,"token":"t","offline_selfdestruct_hours":48},"msg":"success"}`
	_, _, _, hours := decodeHeartbeatResponse(t, body)
	if hours == nil || *hours != 48 {
		t.Fatalf("hours = %v", hours)
	}
}

// 关键回归：按顶层解析不报错但静默为空——线上 device_id 丢失正是此因。
func TestHeartbeatTopLevelSilentlyEmpty(t *testing.T) {
	body := `{"code":0,"data":{"action":"ok","device_id":26,"token":"tok"},"msg":"success"}`
	var flat struct {
		DeviceID interface{} `json:"device_id"`
	}
	if err := json.Unmarshal([]byte(body), &flat); err != nil {
		t.Fatalf("顶层解析不应报错: %v", err)
	}
	if flat.DeviceID != nil {
		t.Fatal("预期顶层静默为空，服务端响应形状可能已变更")
	}
	if id, _, _, _ := decodeHeartbeatResponse(t, body); id != "26" {
		t.Fatalf("信封解析必须拿到 device_id，实际 %q", id)
	}
}

// 错误信封（code!=0, data=null）不能panic，且不应产出 device_id。
func TestHeartbeatErrorEnvelope(t *testing.T) {
	body := `{"code":1,"data":null,"msg":"设备令牌无效"}`
	if id, _, _, _ := decodeHeartbeatResponse(t, body); id != "" {
		t.Fatalf("错误信封不应有 device_id，实际 %q", id)
	}
}
