package notifsvc

import (
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/rama/b-wise/notify/internal/domain/entity"
)

var errNoSubscriptions = errors.New("tidak ada langganan push aktif")

// Browser-provided endpoints must point at a known public push provider.
func validateSubscription(endpoint, p256dh, auth string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return fmt.Errorf("endpoint push tidak valid")
	}
	host := strings.ToLower(u.Hostname())
	allowed := host == "fcm.googleapis.com" || host == "updates.push.services.mozilla.com" || host == "web.push.apple.com" || strings.HasSuffix(host, ".notify.windows.com")
	if !allowed {
		return fmt.Errorf("penyedia push tidak didukung")
	}
	key, err := base64.RawURLEncoding.DecodeString(p256dh)
	if err != nil {
		return fmt.Errorf("kunci push tidak valid")
	}
	if _, err = ecdh.P256().NewPublicKey(key); err != nil {
		return fmt.Errorf("kunci push tidak valid")
	}
	secret, err := base64.RawURLEncoding.DecodeString(auth)
	if err != nil || len(secret) != 16 {
		return fmt.Errorf("auth push tidak valid")
	}
	return nil
}

func (s *Service) PublicPushKey() string {
	if s.vapidPrivate == "" {
		return ""
	}
	return s.vapidPublic
}

func pushPayload(o *entity.Outbox) ([]byte, error) {
	data := map[string]interface{}{}
	if o.Payload != "" {
		if err := json.Unmarshal([]byte(o.Payload), &data); err != nil {
			return nil, err
		}
	}
	if data == nil {
		data = map[string]interface{}{}
	}
	data["title"], data["body"], data["tag"] = o.Title, o.Body, o.ID
	target, _ := data["url"].(string)
	if !strings.HasPrefix(target, "/dashboard") || strings.ContainsAny(target, "\\\r\n") {
		target = "/dashboard"
	}
	data["url"] = target
	return json.Marshal(data)
}

func (s *Service) sendPush(o *entity.Outbox) error {
	var subs []entity.PushSubscription
	if err := s.db.Where("user_id = ?", o.Recipient).Find(&subs).Error; err != nil {
		return err
	}
	if len(subs) == 0 {
		return errNoSubscriptions
	}
	if s.PublicPushKey() == "" {
		return fmt.Errorf("push VAPID belum dikonfigurasi")
	}
	payload, err := pushPayload(o)
	if err != nil {
		return err
	}
	delivered := []string{}
	if o.Delivered != "" {
		if err = json.Unmarshal([]byte(o.Delivered), &delivered); err != nil {
			return err
		}
	}
	done := map[string]bool{}
	for _, id := range delivered {
		done[id] = true
	}
	var lastErr error
	accepted := len(delivered)
	for _, sub := range subs {
		if done[sub.ID] {
			continue
		}
		if err = validateSubscription(sub.Endpoint, sub.P256DH, sub.Auth); err != nil {
			lastErr = err
			continue
		}
		resp, sendErr := webpush.SendNotification(payload, &webpush.Subscription{Endpoint: sub.Endpoint, Keys: webpush.Keys{Auth: sub.Auth, P256dh: sub.P256DH}}, &webpush.Options{HTTPClient: s.client, Subscriber: s.vapidSubject, VAPIDPublicKey: s.vapidPublic, VAPIDPrivateKey: s.vapidPrivate, TTL: 3600})
		if sendErr != nil {
			lastErr = sendErr
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == 404 || resp.StatusCode == 410 {
			if err = s.db.Delete(&sub).Error; err != nil {
				lastErr = err
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("push service HTTP %d", resp.StatusCode)
			continue
		}
		delivered = append(delivered, sub.ID)
		encoded, _ := json.Marshal(delivered)
		if err = s.db.Model(o).Update("delivered", string(encoded)).Error; err != nil {
			return err
		}
		accepted++
	}
	if lastErr != nil {
		return lastErr
	}
	if accepted == 0 {
		return errNoSubscriptions
	}
	return nil
}
