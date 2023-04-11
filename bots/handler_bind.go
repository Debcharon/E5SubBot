package bots

import (
	"fmt"
	"github.com/iyear/E5SubBot/config"
	"github.com/iyear/E5SubBot/model"
	"github.com/iyear/E5SubBot/pkg/microsoft"
	"github.com/iyear/E5SubBot/service/srv_client"
	"github.com/iyear/E5SubBot/util"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
	tb "gopkg.in/tucnak/telebot.v2"
	"strconv"
	"strings"
)

func bBind(m *tb.Message) {
	bot.Send(m.Chat,
		fmt.Sprintf("Register application [Directly](%s)", microsoft.GetRegURL()),
		tb.ModeMarkdown,
	)

	bot.Send(m.Chat,
		"Please reply with your `client_id` + `client_secret`",
		&tb.SendOptions{ParseMode: tb.ModeMarkdown,
			ReplyMarkup: &tb.ReplyMarkup{ForceReply: true}},
	)

	UserStatus[m.Chat.ID] = StatusBind1
	UserClientId[m.Chat.ID] = m.Text
}

func bBind1(m *tb.Message) {
	if !m.IsReply() {
		bot.Send(m.Chat, "WARN: Please reply to message to bind")
		return
	}
	tmp := strings.Split(m.Text, " ")
	if len(tmp) != 2 {
		bot.Send(m.Chat, "WARN: Wrong format")
		return
	}
	id := tmp[0]
	secret := tmp[1]
	bot.Send(m.Chat,
		fmt.Sprintf("Authorized Account [Directly](%s)", microsoft.GetAuthURL(id)),
		tb.ModeMarkdown,
	)

	bot.Send(m.Chat,
	"WARN: Please reply http://localhost/... + alias",
		&tb.SendOptions{ParseMode: tb.ModeMarkdown,
			ReplyMarkup: &tb.ReplyMarkup{ForceReply: true},
		},
	)
	UserStatus[m.Chat.ID] = StatusBind2
	UserClientId[m.Chat.ID] = id
	UserClientSecret[m.Chat.ID] = secret
}

func bBind2(m *tb.Message) {
	if !m.IsReply() {
		bot.Send(m.Chat, "WARN: Please reply to message to bind")
		return
	}
	if len(srv_client.GetClients(m.Chat.ID)) == config.BindMaxNum {
		bot.Send(m.Chat, "WARN: You have reached maximum binding number")
		return
	}
	bot.Send(m.Chat, "Binding now...")

	tmp := strings.Split(m.Text, " ")
	if len(tmp) != 2 {
		bot.Send(m.Chat, "Wrong format")
	}
	code := util.GetURLValue(tmp[0], "code")
	alias := tmp[1]

	id := UserClientId[m.Chat.ID]
	secret := UserClientSecret[m.Chat.ID]

	refresh, err := microsoft.GetTokenWithCode(id, secret, code)
	if err != nil {
		bot.Send(m.Chat, fmt.Sprintf("Cannot get RefreshToken ERROR:%s", err))
		return
	}
	bot.Send(m.Chat, "Successfully to get Token")

	refresh, info, err := microsoft.GetUserInfo(id, secret, refresh)
	if err != nil {
		bot.Send(m.Chat, fmt.Sprintf("Cannot get user info ERROR:%s", err))
		return
	}
	c := &model.Client{
		TgId:         m.Chat.ID,
		RefreshToken: refresh,
		MsId:         util.Get16MD5Encode(gjson.Get(info, "id").String()),
		Alias:        alias,
		ClientId:     id,
		ClientSecret: secret,
		Other:        "",
	}

	if srv_client.IsExist(c.TgId, c.ClientId) {
		bot.Send(m.Chat, "WARN: The application has already been bound, no need to bind again")
		return
	}

	bot.Send(m.Chat,
	fmt.Sprintf("ms_id: %s\nuserPrincipalName: %s\ndisplayName: %s",
			c.MsId,
			gjson.Get(info, "userPrincipalName").String(),
			gjson.Get(info, "displayName").String(),
		),
	)

	if err = srv_client.Add(c); err != nil {
		bot.Send(m.Chat, "ERROR: User data fail to write in database")
		return
	}

	bot.Send(m.Chat, "Bind account successfully!")
	delete(UserStatus, m.Chat.ID)
	delete(UserClientId, m.Chat.ID)
	delete(UserClientSecret, m.Chat.ID)
}

func bUnBind(m *tb.Message) {
	var inlineKeys [][]tb.InlineButton
	clients := srv_client.GetClients(m.Chat.ID)

	for _, u := range clients {
		inlineBtn := tb.InlineButton{
			Unique: "unbind" + strconv.Itoa(u.ID),
			Text:   u.Alias,
			Data:   strconv.Itoa(u.ID),
		}
		bot.Handle(&inlineBtn, bUnBindInlineBtn)
		inlineKeys = append(inlineKeys, []tb.InlineButton{inlineBtn})
	}

	bot.Send(m.Chat,
	fmt.Sprintf("Select an account to unbind\n\nCurrent number of bindings: %d/%d", len(srv_client.GetClients(m.Chat.ID)), config.BindMaxNum),
		&tb.ReplyMarkup{InlineKeyboard: inlineKeys},
	)
}
func bUnBindInlineBtn(c *tb.Callback) {
	id, _ := strconv.Atoi(c.Data)
	if err := srv_client.Del(id); err != nil {
		zap.S().Errorw("failed to delete db data",
			"error", err,
			"id", c.Data,
		)
		bot.Send(c.Message.Chat, "ERROR: Fail to unbind account")
		return
	}
	bot.Send(c.Message.Chat, "Unbind account successfully!")
	bot.Respond(c)
}
