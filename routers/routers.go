package routers

import (
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	humanize "github.com/dustin/go-humanize"
	"github.com/gin-gonic/gin"
	"github.com/microcosm-cc/bluemonday"
	"github.com/recoilme/tgram/models"
	"github.com/recoilme/tgram/utils"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/text/language"
	"gopkg.in/russross/blackfriday.v2"
)

//The SiteConfig struct stores site customisations
type siteConfig struct {
	Title       string
	Description string
	Admin       string
	SiteName    string
	AboutPage   string
	Domain      string
}

var (
	// NBSecretPassword some random string
	NBSecretPassword = "A String Very Very Very Niubilty!!@##$!@#4"
	Config           siteConfig
)

const (
	CookieTime = 2592000
)

// CheckAuth - general hook sets all param like lang, user
func CheckAuth() gin.HandlerFunc {
	return func(c *gin.Context) {

		c.Set("config", Config)
		c.Set("path", c.Request.URL.Path)
		var lang = "en"
		var found bool
		acceptedLang := []string{"de", "en", "fr", "ko", "pt", "ru", "sv", "tr", "us", "zh", "tst", "sub", "bs"}
		var tokenStr, username, image string

		hosts := strings.Split(c.Request.Host, ".")
		var host = hosts[0]
		if host == "localhost:8081" {
			// dev
			c.Redirect(http.StatusFound, "http://sub."+host)
			return
		}
		if host == "tgr" {
			// tgr.am
			//fmt.Println("host:tgr")
			t, _, err := language.ParseAcceptLanguage(c.Request.Header.Get("Accept-Language"))
			if err == nil && len(t) > 0 {
				if len(t[0].String()) >= 2 {
					// some lang found
					if len(t[0].String()) == 2 || len(t[0].String()) == 3 {
						// some lang 3 char
						lang = t[0].String()
					} else {
						// remove country code en-US
						langs := strings.Split(t[0].String(), "-")
						if len(langs[0]) == 2 || len(langs[0]) == 3 {
							lang = langs[0]
						}
					}
				}
			}
			for _, v := range acceptedLang {
				if v == lang {
					found = true
					break
				}
			}
			if !found {
				lang = "en"
			}
			// redirect on subdomain
			c.Redirect(http.StatusFound, "http://"+lang+".tgr.am")
			return
		}
		if len(host) < 2 || len(host) > 3 {
			c.Redirect(http.StatusFound, "http://"+lang+".tgr.am")
			return
		}

		for _, v := range acceptedLang {
			if v == host {
				found = true
				break
			}
		}
		if !found {
			c.Redirect(http.StatusFound, "http://"+lang+".tgr.am")
			return
		}

		// store subdomain
		c.Set("lang", host)

		//fmt.Println("lang:", lang, "host:", host, "path", c.Request.URL.Path)

		// token from cookie
		if tokenС, err := c.Cookie("token"); err == nil && tokenС != "" {
			tokenStr = tokenС
		}

		if tokenStr == "" {
			// token from header
			authStr := c.Request.Header.Get("Authorization")
			// Strips 'TOKEN ' prefix from token string
			if len(authStr) > 5 && strings.ToUpper(authStr[0:6]) == "TOKEN " {
				tokenStr = authStr[6:]
			}
		}
		if tokenStr != "" {
			token, tokenErr := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
				// Don't forget to validate the alg is what you expect:
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
				}

				// hmacSampleSecret is a []byte containing your secret, e.g. []byte("my_secret_key")
				return []byte(NBSecretPassword), nil
			})
			if tokenErr == nil && token != nil {
				//token found
				if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
					username = claims["username"].(string)
					image = claims["image"].(string)
				}
			}
		}
		c.Set("username", username)
		c.Set("image", image)
	}
}

// ToStr convert object to string
func ToStr(value interface{}) string {
	return fmt.Sprintf("%s", value)
}

// ToDate convert time 2 date
func ToDate(t time.Time) string {
	return fmt.Sprintf("%s", humanize.Time(t))
}

// GetLead return first paragraph
func GetLead(s string) string {
	if len(s) < 300 {
		return s + ".."
	}
	var delim = strings.IndexRune(s, '\n')
	if delim > 300 || delim < 0 {
		var len = len([]rune(s))
		if len > 300 {
			len = 300
		}
		delim = strings.LastIndexByte(s[:len], ' ')
		if delim < 0 {
			delim = len
		}
	}
	//log.Println(s, delim)
	return s[:delim] + ".."
}

// Home - main page
func Home(c *gin.Context) {

	username := c.GetString("username")
	var users []models.User
	var mentions []models.Mention
	if username != "" {
		users = models.IFollow(c.GetString("lang"), "fol", username)
		mentions = models.Mentions(c.GetString("lang"), username)
	}
	c.Set("users", users)
	c.Set("mentions", mentions)
	if len(users) > 0 || len(mentions) > 0 {
		c.Set("personal", true)
	}
	c.HTML(http.StatusOK, "home.html", c.Keys)
}

// All all page
func All(c *gin.Context) {
	articles, page, prev, next, last, err := models.AllArticles(c.GetString("lang"), c.Query("p"), c.Query("tag"))
	if err != nil {
		renderErr(c, err)
		return
	}
	//log.Println(len(articles))
	c.Set("articles", articles)
	if c.Query("tag") == "" {
		c.Set("page", page)
		c.Set("prev", prev)
		c.Set("next", next)
		c.Set("last", last)
		from_int, _ := strconv.Atoi(c.Query("p"))
		c.Set("p", from_int)
	}
	c.HTML(http.StatusOK, "all.html", c.Keys)
}

func Top(c *gin.Context) {
	articles, err := models.TopArticles(c.GetString("lang"), uint32(20), "plus")
	if err != nil {
		renderErr(c, err)
		return
	}
	c.Set("articles", articles)
	c.HTML(http.StatusOK, "all.html", c.Keys)
}

func Btm(c *gin.Context) {
	articles, err := models.TopArticles(c.GetString("lang"), uint32(20), "minus")
	if err != nil {
		renderErr(c, err)
		return
	}
	c.Set("articles", articles)
	c.HTML(http.StatusOK, "all.html", c.Keys)
}

func renderErr(c *gin.Context, err error) {
	switch c.Request.Header.Get("Accept") {
	case "application/json":
		// Respond with JSON
		c.JSON(http.StatusUnprocessableEntity, err)
	default:
		// Respond with HTML
		c.Set("err", err)
		c.HTML(http.StatusBadRequest, "err.html", c.Keys)
	}
}

func genToken(username, image string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": username,
		"image":    image,
	})

	// Sign and get the complete encoded token as a string using the secret
	return token.SignedString([]byte(NBSecretPassword))
}

// Register page
func Register(c *gin.Context) {
	var err error
	switch c.Request.Method {
	case "GET":
		c.HTML(http.StatusOK, "register.html", c.Keys)
	case "POST":
		ip := c.ClientIP()

		wait := models.RegisterIPGet(ip) //ratelimit(ip, RateIP)
		if wait > 0 {
			e := fmt.Sprintf("Rate limit on registration from your ip, please wait: %d Seconds", wait)
			renderErr(c, errors.New(e))
			return
		}

		type RegisterForm struct {
			Username string `form:"username" json:"username" binding:"exists,alphanum,min=1,max=20"`
			Password string `form:"password" json:"password" binding:"exists,min=6,max=255"`
			Privacy  string `form:"privacy" json:"privacy" `
			Terms    string `form:"terms" json:"terms" `
			Good     string `form:"good" json:"good" ` // This is to avoid spammers
		}
		var rf RegisterForm
		err = c.ShouldBind(&rf)

		if rf.Good == "good" {
			err = errors.New("Error: Sorry, we do not endorse spammers...")
		}
		if rf.Privacy != "privacy" {
			err = errors.New("Error: You must read and accept our Privacy Statement")
		}
		if rf.Terms != "terms" {
			err = errors.New("Error: You must read and accept our Terms of Service")
		}
		if err != nil {
			renderErr(c, err)
			return
		}

		var u models.User
		u.Username = rf.Username
		u.Password = rf.Password

		// create user
		u.Username = strings.ToLower(u.Username)
		u.Lang = c.GetString("lang")
		u.IP = ip
		err = models.UserNew(&u)
		if err != nil {
			renderErr(c, err)
			return
		}

		tokenString, err := genToken(u.Username, "")
		if err != nil {
			renderErr(c, err)
			return
		}
		// add to cache on success
		models.RegisterIPSet(ip)
		//cc.Set(ip, time.Now().Unix(), cache.DefaultExpiration)

		c.SetCookie("token", tokenString, CookieTime, "/", "", false, true)
		switch c.Request.Header.Get("Accept") {
		case "application/json":
			c.JSON(http.StatusCreated, u)
			return
		default:
			c.Redirect(http.StatusFound, "/")
		}

	default:
		c.HTML(http.StatusNotFound, "err.html", errors.New("not found"))
	}
}

// Settings page
func Settings(c *gin.Context) {

	switch c.Request.Method {
	case "GET":
		user, err := models.UserGet(c.GetString("lang"), c.GetString("username"))
		if err != nil {
			renderErr(c, err)
			return
		}
		c.Set("bio", user.Bio)
		c.Set("email", user.Email)
		c.Set("image", user.Image)
		c.HTML(http.StatusOK, "settings.html", c.Keys)
	case "POST":
		var u models.User
		var err error
		u.Username = c.GetString("username")
		err = c.ShouldBind(&u)
		if err != nil {
			renderErr(c, err)
			return
		}
		u.Lang = c.GetString("lang")
		user, err := models.UserCheckGet(u.Lang, u.Username, u.Password)
		if err != nil {
			renderErr(c, err)
			return
		}
		//fmt.Printf("user:%+v\n", u)
		if u.NewPassword != "" {
			bytePassword := []byte(u.NewPassword)
			// Make sure the second param `bcrypt generator cost` between [4, 32)
			passwordHash, _ := bcrypt.GenerateFromPassword(bytePassword, bcrypt.DefaultCost)
			u.Password = ""
			u.NewPassword = ""
			u.PasswordHash = string(passwordHash)
			err = models.UserSave(&u)
			if err != nil {
				renderErr(c, err)
				return
			}
			//logout
			c.SetCookie("token", "", 0, "/", "", false, true)
			c.Redirect(http.StatusFound, "/")
			return
		} else {
			u.Password = ""
			u.PasswordHash = user.PasswordHash
			err = models.UserSave(&u)
		}

		if err != nil {
			renderErr(c, err)
			return
		}
		if u.Image != user.Image {
			// upd token
			tokenString, err := genToken(u.Username, u.Image)
			if err != nil {
				renderErr(c, err)
				return
			}
			if c.Request.Header.Get("Accept") != "application/json" {
				c.SetCookie("token", tokenString, CookieTime, "/", "", false, true)
			}

		}
		switch c.Request.Header.Get("Accept") {
		case "application/json":
			c.JSON(http.StatusOK, u)
			return
		default:
			c.Redirect(http.StatusFound, "/")
		}
	}
}

// Logout remove cookie
func Logout(c *gin.Context) {
	switch c.Request.Method {
	case "GET":
		c.SetCookie("token", "", 0, "/", "", false, true)
		c.Redirect(http.StatusFound, "/")
	case "POST":
	}
}

// Login page
func Login(c *gin.Context) {
	var err error
	switch c.Request.Method {
	case "GET":
		c.HTML(http.StatusOK, "login.html", c.Keys)
	case "POST":
		var u models.User
		err = c.ShouldBind(&u)
		if err != nil {
			renderErr(c, err)
			return
		}

		user, err := models.UserCheckGet(c.GetString("lang"), u.Username, u.Password)
		if err != nil {
			renderErr(c, err)
			return
		}
		tokenString, err := genToken(user.Username, user.Image)
		if err != nil {
			renderErr(c, err)
			return
		}
		c.SetCookie("token", tokenString, CookieTime, "/", "", false, true)
		switch c.Request.Header.Get("Accept") {
		case "application/json":
			c.JSON(http.StatusOK, user)
		default:
			c.Redirect(http.StatusFound, "/")
		}

	}
}

// Editor page
func Editor(c *gin.Context) {
	aid, _ := strconv.Atoi(c.Param("aid"))
	c.Set("aid", aid)
	//postRate := c.GetString("lang") + ":p:" + c.GetString("username")
	switch c.Request.Method {
	case "GET":

		if aid > 0 {
			// check username
			username := c.GetString("username")
			a, err := models.ArticleGet(c.GetString("lang"), username, uint32(aid))
			if err != nil {
				renderErr(c, err)
				return
			}
			str := strings.Replace(a.Body, "\n\n", "\r\n", -1)
			c.Set("body", str)
			c.Set("title", a.Title)
			c.Set("ogimage", a.OgImage)
			c.Set("tag", a.Tag)
		} else {
			wait := models.PostLimitGet(c.GetString("lang"), c.GetString("username")) //ratelimit(postRate, RatePost)
			if wait > 0 {
				e := fmt.Sprintf("Rate limit for new users on new post, please wait: %d Seconds", wait)
				renderErr(c, errors.New(e))
				return
			}
		}
		c.HTML(http.StatusOK, "article_edit.html", c.Keys)
	case "POST":
		//log.Println("aid", aid)
		var err error
		var abind models.Article
		err = c.ShouldBind(&abind)
		if err != nil {
			renderErr(c, err)
			return
		}
		username := c.GetString("username")
		lang := c.GetString("lang")
		host := "http://" + c.Request.Host + "/"

		body, err := models.ImgProcess(strings.Replace(strings.TrimSpace(abind.Body), "\r\n", "\n\n", -1), lang, username, host)
		if err != nil {
			renderErr(c, err)
			return
		}
		readingTime, wordCount := utils.ReadingTime(body)
		unsafe := blackfriday.Run([]byte(body))
		html := template.HTML(bluemonday.UGCPolicy().SanitizeBytes(unsafe))
		tag := strings.TrimSpace(abind.Tag)
		title := strings.TrimSpace(abind.Title)
		ogimage := strings.TrimSpace(abind.OgImage)
		//log.Printf("ogimage:'%s'\n", ogimage)
		var a models.Article
		if aid > 0 {

			a, err := models.ArticleGet(lang, username, uint32(aid))
			if err != nil {
				renderErr(c, err)
				return
			}
			a.HTML = html
			a.Body = body
			a.Title = title
			a.OgImage = ogimage
			a.ReadingTime = readingTime
			a.WordCount = wordCount
			oldTag := a.Tag
			a.Tag = tag
			err = models.ArticleUpd(a, oldTag)
			if err != nil {
				renderErr(c, err)
				return
			}
			//log.Println("aid2", a)
			//log.Println("Author", a.Author, "a.ID", a.ID, fmt.Sprintf("/@%s/%d", a.Author, a.ID))
			c.Redirect(http.StatusFound, fmt.Sprintf("/@%s/%d", a.Author, a.ID))
			return
		}
		wait := models.PostLimitGet(c.GetString("lang"), c.GetString("username")) //ratelimit(postRate, RatePost)
		if wait > 0 {
			e := fmt.Sprintf("Rate limit for new users on new post, please wait: %d Seconds", wait)
			renderErr(c, errors.New(e))
			return
		}
		if models.UserBanGet(username) { //_, bannedAuthor := cc.Get("ban:uid:" + username); bannedAuthor {
			renderErr(c, errors.New("You are banned for 24 h for spam, advertising, illegal and / or copyrighted content. Sorry about that("))
			return
		}
		/*
			if _, bannedIP := cc.Get(c.ClientIP()); bannedIP {
				renderErr(c, errors.New("This ip was banned for 24 h for spam, advertising, illegal and / or copyrighted content. Sorry about that("))
				return
			}*/
		a.Lang = lang
		a.Author = username
		a.Image = c.GetString("image")
		a.OgImage = ogimage
		a.CreatedAt = time.Now()
		a.HTML = html
		a.Body = body
		a.Title = title
		a.ReadingTime = readingTime
		a.WordCount = wordCount
		a.Tag = tag
		newaid, err := models.ArticleNew(&a)
		if err != nil {
			renderErr(c, err)
			return
		}
		a.ID = newaid
		// add to cache on success
		models.PostLimitSet(c.GetString("lang"), c.GetString("username"))
		//cc.Set(postRate, time.Now().Unix(), cache.DefaultExpiration)

		//log.Println("Author", a.Author, "a.ID", a.ID, fmt.Sprintf("/@%s/%d", a.Author, a.ID))
		c.Redirect(http.StatusFound, fmt.Sprintf("/@%s/%d", a.Author, a.ID))
		return
	}
}

// Article page
func Article(c *gin.Context) {
	switch c.Request.Method {
	case "GET":
		lang := c.GetString("lang")
		aid, _ := strconv.Atoi(c.Param("aid"))
		aid32 := models.Uint32toBin(uint32(aid))
		username := c.Param("username")
		a, err := models.ArticleGet(lang, username, uint32(aid))
		if err != nil {
			renderErr(c, err)
			return
		}
		path := c.GetString("path")
		if username != "" {
			url := "/@" + username + "/" + c.Param("aid")
			models.MentionDel(lang, c.GetString("username"), url)
		}
		c.Set("link", "http://"+c.Request.Host+path)
		c.Set("article", a)
		c.Set("title", a.Title)
		c.Set("description", GetLead(a.Body))
		c.Set("body", a.HTML)
		isFolow := models.IsFollowing(lang, "fol", username, c.GetString("username"))
		c.Set("isfollow", isFolow)
		followcnt := models.FollowCount(lang, "fol", username)
		c.Set("followcnt", followcnt)

		// fav
		isFav := models.IsFollowing(lang, "fav", string(aid32), c.GetString("username"))
		c.Set("isfav", isFav)
		favcnt := models.FollowCount(lang, "fav", string(aid32))
		c.Set("favcnt", favcnt)
		//log.Println("Art", a)

		// view counter
		view := models.ArticleViewGet(lang, c.ClientIP(), a.ID)
		c.Set("view", view)
		c.Set("ogimage", a.OgImage)
		c.HTML(http.StatusOK, "article.html", c.Keys)
		//c.JSON(http.StatusOK, a)
	}
}

// ArticleDelete delete page by id of current user
func ArticleDelete(c *gin.Context) {
	switch c.Request.Method {
	case "GET":
		aid, _ := strconv.Atoi(c.Param("aid"))
		username := c.GetString("username")
		err := models.ArticleDelete(c.GetString("lang"), username, uint32(aid))
		if err != nil {
			renderErr(c, err)
			return
		}
		// remove rate limit on delete
		models.PostLimitDel(c.GetString("lang"), username)
		//cc.Delete(c.GetString("lang") + ":p:" + username)
		c.Redirect(http.StatusFound, "/")
	}
}

// Follow subscribe on user
func Follow(c *gin.Context) {
	switch c.Request.Method {
	case "GET":
		user := c.Param("user")
		username := c.GetString("username")
		action := c.Param("action")
		err := models.Following(c.GetString("lang"), "fol", user, username)
		if err != nil {
			renderErr(c, err)
			return
		}
		c.Redirect(http.StatusFound, action)
	}
}

// Unfollow unsubscribe
func Unfollow(c *gin.Context) {
	switch c.Request.Method {
	case "GET":
		user := c.Param("user")
		action := c.Param("action")
		err := models.Unfollowing(c.GetString("lang"), "fol", user, c.GetString("username"))
		if err != nil {
			renderErr(c, err)
			return
		}
		c.Redirect(http.StatusFound, action)
	}
}

// Fav add to favorites
func Fav(c *gin.Context) {
	switch c.Request.Method {
	case "GET":

		aid, _ := strconv.Atoi(c.Param("aid"))
		aid32 := models.Uint32toBin(uint32(aid))
		//fmt.Println(aid32, string(aid32), []byte(string(aid32)))
		action := c.Param("action")
		username := c.GetString("username")

		err := models.Following(c.GetString("lang"), "fav", string(aid32), username)
		if err != nil {
			renderErr(c, err)
			return
		}
		c.Redirect(http.StatusFound, action)
	}
}

// Unfav remove from favorites
func Unfav(c *gin.Context) {
	switch c.Request.Method {
	case "GET":
		aid, _ := strconv.Atoi(c.Param("aid"))
		aid32 := models.Uint32toBin(uint32(aid))
		action := c.Param("action")
		err := models.Unfollowing(c.GetString("lang"), "fav", string(aid32), c.GetString("username"))
		if err != nil {
			renderErr(c, err)
			return
		}
		c.Redirect(http.StatusFound, action)
	}
}

// GoToRegister redirect to registration
func GoToRegister() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString("username") == "" {
			c.Redirect(http.StatusFound, "/register")
			c.Abort()
		}
	}
}

// Author page
func Author(c *gin.Context) {
	authorStr := c.Param("username")
	lang := c.GetString("lang")
	author, err := models.UserGet(lang, authorStr)
	if err != nil {
		renderErr(c, err)
		return
	}

	articles, page, prev, next, last, err := models.ArticlesAuthor(c.GetString("lang"), c.GetString("username"), authorStr, c.Query("p"))
	if err != nil {
		renderErr(c, err)
		return
	}

	c.Set("articles", articles)
	c.Set("page", page)
	c.Set("prev", prev)
	c.Set("next", next)
	c.Set("last", last)
	from_int, _ := strconv.Atoi(c.Query("p"))
	c.Set("p", from_int)

	c.Set("author", author)
	isFolow := models.IsFollowing(lang, "fol", authorStr, c.GetString("username"))
	c.Set("isfollow", isFolow)
	followcnt := models.FollowCount(lang, "fol", authorStr)
	c.Set("followcnt", followcnt)

	// fav
	aid, _ := strconv.Atoi(c.Param("aid"))
	aid32 := models.Uint32toBin(uint32(aid))
	isFav := models.IsFollowing(lang, "fav", string(aid32), c.GetString("username"))
	c.Set("isfav", isFav)
	favcnt := models.FollowCount(lang, "fav", string(aid32))
	c.Set("favcnt", favcnt)
	c.HTML(http.StatusOK, "author.html", c.Keys)
}

// CommentNew create comment
func CommentNew(c *gin.Context) {
	switch c.Request.Method {
	case "POST":
		lang := c.GetString("lang")
		aid, _ := strconv.Atoi(c.Param("aid"))
		username := c.Param("username")

		var err error
		var a models.Article
		err = c.ShouldBind(&a)
		if err != nil {
			renderErr(c, err)
			return
		}
		//rateComKey := lang + ":c:" + c.GetString("username")
		wait := models.ComLimitGet(lang, c.GetString("username")) //ratelimit(rateComKey, RateComment)
		if wait > 0 {
			e := fmt.Sprintf("Rate limit for new users on new comment, please wait: %d Seconds", wait)
			renderErr(c, errors.New(e))
			return
		}

		parsed := models.ReplyParse(strings.Replace(a.Body, "\r\n", "\n\n", -1), lang)
		a.Body = parsed
		//log.Println("bod", a.Body)
		unsafe := blackfriday.Run([]byte(a.Body))
		html := bluemonday.UGCPolicy().SanitizeBytes(unsafe)
		a.HTML = template.HTML(html)

		a.Lang = lang
		a.Author = c.GetString("username")
		a.Image = c.GetString("image")
		a.CreatedAt = time.Now()

		//a.Body = string(body)
		cid, err := models.CommentNew(&a, username, uint32(aid))
		if err != nil {
			renderErr(c, err)
			return
		}
		ment := GetLead(a.Body)
		url := "/@" + username + "/" + c.Param("aid")
		fullurl := url + "#comment" + strconv.Itoa(int(cid))
		models.MentionNew(a.Body, lang, ment, a.Author, url, fullurl, uint32(aid), cid)

		// add to cache on success
		models.ComLimitSet(lang, c.GetString("username"))
		//cc.Set(rateComKey, time.Now().Unix(), cache.DefaultExpiration)
		c.Redirect(http.StatusFound, fmt.Sprintf("/@%s/%d#comment%d", username, aid, cid))
		//c.JSON(http.StatusCreated, a) //gin.H{"article": serializer.Response()})
		//c.Redirect(http.Sta
	}
}

// Favorites return last 100 fav
func Favorites(c *gin.Context) {
	switch c.Request.Method {
	case "GET":
		lang := c.GetString("lang")
		user := c.Param("username")
		articles := models.Favorites(lang, user)
		c.Set("articles", articles)
		var prev, next, last uint32
		page := ""
		c.Set("page", page)
		c.Set("prev", prev)
		c.Set("next", next)
		c.Set("last", last)
		//from_int, _ := strconv.Atoi(c.Query("p"))
		from_int := 0
		c.Set("p", from_int)
		c.HTML(http.StatusOK, "all.html", c.Keys)
	}
}

// ArticleBad - delete article and ban author
func ArticleBad(c *gin.Context) {
	switch c.Request.Method {
	case "GET":
		aid, _ := strconv.Atoi(c.Param("aid"))
		author := c.Param("author")
		username := c.GetString("username")

		//check for me
		if username != "recoilme" {
			renderErr(c, errors.New("You are not recoilme"))
			return
		}
		// check for not me
		if author == "recoilme" {
			renderErr(c, errors.New("You are recoilme!"))
			return
		}
		err := models.ArticleDelete(c.GetString("lang"), author, uint32(aid))
		if err != nil {
			renderErr(c, err)
			return
		}
		// remove rate limit on delete
		//cc.Delete(c.GetString("lang") + ":p:" + username)
		models.UserBanSet(author)
		//cc.Set("ban:uid:"+author, time.Now().Unix(), cache.DefaultExpiration)
		//}
		c.Redirect(http.StatusFound, "/@"+author)
	}
}

// Upload image
func Upload(c *gin.Context) {
	switch c.Request.Method {
	case "GET":
		c.HTML(http.StatusOK, "upload.html", c.Keys)
	case "POST":
		var fileHeader *multipart.FileHeader
		var err error
		var src multipart.File
		var newElement = ""
		minSize := 102400
		if fileHeader, err = c.FormFile("file"); err != nil {
			renderErr(c, err)
			return
		}
		//log.Println("Size:", fileHeader.Size)
		if fileHeader.Size > int64(minSize*100) {
			renderErr(c, errors.New("File too big"))
			return
		}

		if src, err = fileHeader.Open(); err == nil {
			defer src.Close()
			//log.Println("src:", src)
			b, err := ioutil.ReadAll(src)
			//log.Println("b:", len(b))
			if err != nil {
				renderErr(c, err)
				return
			}
			file, orig, origSize := models.Store("", c.GetString("lang"), c.GetString("username"), b)
			if file == "" || orig == "" {
				renderErr(c, errors.New("Some error"))
				return
			}
			//TODO https
			host := "http://" + c.Request.Host + "/"

			if origSize > minSize {
				newElement = "[![](" + host + file + ")](" + host + orig + ")"
			} else {
				newElement = "![](" + host + orig + ")"
			}

			c.Set("newelement", newElement)
			c.HTML(http.StatusOK, "upload.html", c.Keys)

		} else {
			//log.Println("open error", err)
			renderErr(c, err)
			return
		}

	}
}

func Policy(c *gin.Context) {
	c.HTML(http.StatusOK, "policy.html", c.Keys)
	return
}

func Terms(c *gin.Context) {
	c.HTML(http.StatusOK, "terms.html", c.Keys)
	return
}

func CommentUp(c *gin.Context) {
	switch c.Request.Method {
	case "GET":

		authorCom := c.Param("authorcom")
		authorArt := c.Param("authorart")
		username := c.GetString("username")
		lang := c.GetString("lang")
		aid := c.Param("aid")
		cid := c.Param("cid")
		//log.Println("user", author, aid, cid)
		if authorCom != username {
			// no myself vote
			err := models.ComUpSet(lang, username, cid)
			if err != nil {
				renderErr(c, err)
				return
			}
			// store vote
			aidint, _ := strconv.Atoi(aid)
			a, err := models.ArticleGet(lang, authorArt, uint32(aidint))
			if err != nil {
				renderErr(c, err)
				return
			}
			//log.Println("a:", a)
			comments := a.Comments
			cidint, _ := strconv.Atoi(cid)

			for ind, com := range comments {
				if com.ID == uint32(cidint) {
					votes := com.Plus
					a.Comments[ind].Plus = votes + 1
					models.ArticleUpd(a, a.Tag)
					break
				}
			}
		} else {
			renderErr(c, errors.New("You may not vote for yourself("))
			return
		}
		c.Redirect(http.StatusFound, fmt.Sprintf("/@%s/%s#comment%s", authorArt, aid, cid))
	}
}

func Vote(c *gin.Context) {
	switch c.Request.Method {
	case "GET":
		mode := c.Param("mode")
		author := c.Param("author")
		username := c.GetString("username")
		lang := c.GetString("lang")
		aid := c.Param("aid")

		//log.Println("author", author, aid, mode, username, lang)

		if author != username {

			err := models.VoteSet(lang, username)
			if err != nil {
				renderErr(c, err)
				return
			}
			// store vote
			aidint, _ := strconv.Atoi(aid)
			a, err := models.ArticleGet(lang, author, uint32(aidint))
			if err != nil {
				renderErr(c, err)
				return
			}
			switch mode {
			case "up":
				a.Plus = a.Plus + 1
			case "down":
				a.Minus = a.Minus + 1
			default:
				renderErr(c, errors.New("Not implemented"))
				return
			}
			models.ArticleUpd(a, a.Tag)
		} else {
			// no myself vote
			renderErr(c, errors.New("You may not vote for yourself("))
			return
		}
		c.Redirect(http.StatusFound, fmt.Sprintf("/@%s/%s#comments", author, aid))
	}
}
