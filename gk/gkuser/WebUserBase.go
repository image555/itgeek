package gkuser

import (
	"github.com/dchest/captcha"
	"strconv"
	"crypto/md5"
	"encoding/hex"
	"time"
	"utils/base"
	"strings"
	"github.com/cihub/seelog"
	"github.com/ecdiy/itgeek/gk/ws"
	"fmt"
)

var tokenMap = make(map[string]map[string]string)

func UserMd5Pass(Username, pass string) string {
	h := md5.New()
	h.Write([]byte( Username + "," + pass))
	return hex.EncodeToString(h.Sum(nil))
}
func WebUserLogin(param *ws.Param, res map[string]interface{}) {
	r := DoUserLogin(param, param.String("Username"), param.String("Password"), param.String("Captcha"), param.String("Digits"), res)
	res["Result"] = r.Result
	res["Status"] = r.Status
}

func DoUserLogin(param *ws.Param, Username, Password, Captcha, Digits string, res map[string]interface{}) *base.Result {
	if len(Username) == 0 || len(Password) == 0 {
		return base.StErrorParameter.Result("Password")
	}
	v, ext, e := ws.UserDao.BaseInfoByUsername(param.SiteId, Username)
	if e != nil {
		return base.StErrorDb.ResultNil()
	}
	if !ext {
		return base.StUsernameNotExist.Result("Username")
	}
	if v == nil || len(v) == 0 {
		return base.StUsernameNotExist.Result("Username")
	}
	errTime, _ := strconv.Atoi(v["PasswordError"])
	if errTime > 3 {
		if Captcha == "" {
			return base.StErrorCaptcha.Result("Captcha").Put("Val", captcha.New())
		} else {
			if !captcha.VerifyString(Captcha, Digits) {
				return base.StErrorCaptcha.Result("Captcha").Put("Val", captcha.New())
			}
		}
	}
	if v["Password"] == UserMd5Pass(Username, Password) {
		ws.UserDao.SetPasswordError(0, Username, param.SiteId, )
		res["Info"] = v
		return UserInfoToRedis(param.SiteId, param.Ua, v, )
	} else {
		ws.UserDao.SetPasswordError(errTime+1, Username, param.SiteId, )
		return base.StUserPassError.Result("Password")
	}
}

func UserInfoToRedis(siteId int64, ua string, v map[string]string) *base.Result {
	delete(v, "Password")
	delete(v, "PasswordError")

	h := md5.New()
	h.Write([]byte(v["Id"] + ";" + strconv.FormatInt(time.Now().UnixNano(), 16)))
	tk := hex.EncodeToString(h.Sum(nil))
	v["Token"] = tk

	result := base.OK.ResultNil()
	result.Result = v["Id"] + "_" + tk
	ws.TokenDao.Del(ua, v["Id"], siteId)
	ws.TokenDao.Add(v["Id"], ua, tk, siteId)
	tokenMap[ua+"_"+v["Id"]] = v
	delete(result.Param, "Token")

	return result
}

//-----

func WebUserRegister(param *ws.Param, res map[string]interface{}) {
	r := doUserRegister(param, res)
	res["Result"] = r.Result
	res["Param"] = r.Param
	res["Status"] = r.Status
}

func doUserRegister(m *ws.Param, res map[string]interface{}) *base.Result {
	captchaId := m.String("CaptchaId")
	captchaVal := m.String("CaptchaVal")
	if captchaId != "" && captchaVal != "" {
		if !captcha.VerifyString(captchaId, captchaVal) {
			return base.StErrorCaptcha.Result(captcha.New())
		}
		Email := m.String("Email")
		Password := m.String("Password")
		Username := m.String("Username")
		Mobile := m.String("Mobile")

		if len(Username) < 5 || strings.Index(Username, "@") > 0 ||
			len(Password) < 6 || strings.Index(Email, "@") < 0 ||
			len(Mobile) < 8 || len(Mobile) > 32 {
			seelog.Warn("参数不合法.", m)
			return base.StErrorParameter.Result(captcha.New())
		}
		uCount, unb, _ := ws.UserDao.CheckByUsername(m.SiteId, Username)
		seelog.Info("~~~UserDao Register~~", Username, Mobile, Email)
		if unb && uCount > 0 {
			return base.StUsernameExist.Result(captcha.New())
		}
		eCount, eb, _ := ws.UserDao.CheckByEmail(m.SiteId, Email)
		if eb && eCount > 0 {
			return base.StEmailExist.Result(captcha.New())
		}
		Password = UserMd5Pass(Username, Password)
		id, ue := ws.UserDao.Add(m.SiteId, Username, Password, Email, Mobile)

		if ue == nil {
			v, _, _ := ws.UserDao.BaseInfo(m.SiteId, id)
			rs := UserInfoToRedis(m.SiteId, m.Ua, v)
			res["Info"] = v
			ws.KvDao.UserCount(m.SiteId, m.SiteId)

			fee := ws.GetSoreRule(m.SiteId).Register
			if fee != 0 {
				UpCount(&ws.UpReq{UserId: id, Fee: fee, EntityId: fmt.Sprint(id), SiteId: m.SiteId,
					ScoreType: "初始资本",
					ScoreDesc: `获得初始资本 ` + fmt.Sprint(fee)})
			}
			return rs
		} else {
			return base.StErrorUnknown.Result(captcha.New())
		}
	} else {
		seelog.Warn("注册参数错误:", m)
		return base.StErrorParameter.Result(captcha.New())
	}
}
