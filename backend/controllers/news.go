package controllers

import (
	"log"
	"news-api/config"
	"news-api/database"
	"news-api/models"
	"news-api/utils/similarity"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
)

var maxNewsNumofPage int = config.MaxNewsNumofPage

const maxMotherNews = 10

func NewsRecommendHandler(c *fiber.Ctx) error {
	id := c.Locals("id").(string)

	var user models.User

	res := database.DB.Where("id = ?", id).First(&user)
	if res.Error != nil {
		c.Status(fiber.ErrBadRequest.Code)
		return c.JSON(fiber.Map{
			"message": "unauthenticated",
		})
	}

	var data []models.News
	if err := database.DB.Limit(maxNewsNumofPage).Model(&user).Association("RecommendNews").Find(&data); err != nil {
		c.Status(fiber.StatusInternalServerError)
		return c.JSON(fiber.Map{
			"message": "internal server error",
		})
	}

	if len(data) == 0 {
		NewsHandlersByCategory("general")(c)
		return nil
	}

	return c.JSON(data)
}

func updateRecommendList(user models.User) error {
	err := database.DB.Raw(
		"SELECT news.* FROM news INNER JOIN liked_news ON news.id=liked_news.news_id WHERE liked_news.user_id=? ORDER BY liked_news.liked_at DESC LIMIT ?",
		user.ID, maxMotherNews).Find(&user.LikedNews).Error
	if err != nil {
		panic(err.Error)
	}

	if len(user.LikedNews) == 0 {
		return nil
	}

	var recentNews []models.News
	database.DB.Model(&models.News{}).Find(&recentNews).Order("created_at DESC").Limit(100)

	r := similarity.NewRecommender(recentNews)
	defer r.Close()

	data := make([]models.News, 0)
	for _, likedNews := range user.LikedNews {
		data = append(data, r.SimOrderNews(likedNews, recentNews)...)
	}

	return database.DB.Model(&user).Association("RecommendNews").Replace(data)
}

func NewsHandlersByCategory(category string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Locals("id").(string)

		var user models.User

		res := database.DB.Where("id = ?", id).First(&user)
		if res.Error != nil {
			c.Status(fiber.ErrBadRequest.Code)
			return c.JSON(fiber.Map{
				"message": "unauthenticated",
			})
		}

		var newsArr []models.News

		database.DB.Where("category = ?", category).Order("created_at").Find(&newsArr)

		return c.JSON(newsArr)
	}
}

func LikeStateHandler(c *fiber.Ctx) error {
	var user models.User
	userID := c.Locals("id").(string)
	intUserID, _ := strconv.Atoi(userID)
	user.ID = uint(intUserID)

	var news models.News

	newsID := c.Query("news_id")
	intNewsID, _ := strconv.Atoi(newsID)
	news.ID = uint(intNewsID)

	database.DB.Model(&user).Association("LikedNews").Find(&user.LikedNews, []int{int(news.ID)})

	state := false
	if len(user.LikedNews) == 1 {
		state = true
	}

	return c.JSON(fiber.Map{
		"state": state,
	})
}

func LikeNewsHandler(c *fiber.Ctx) error {
	userID := c.Locals("id").(string)
	var user models.User
	res := database.DB.Where("id = ?", userID).First(&user)
	if res.Error != nil {
		c.Status(fiber.ErrBadRequest.Code)
		return c.JSON(fiber.Map{
			"message": res.Error.Error(),
		})
	}

	newsID := c.Query("news_id")
	var news models.News
	res = database.DB.Where("id = ?", newsID).First(&news)
	if res.Error != nil {
		c.Status(fiber.ErrBadRequest.Code)
		return c.JSON(fiber.Map{
			"message": res.Error.Error(),
		})
	}

	action := c.Query("action")

	var err error
	if action == "do" {
		database.DB.Create(&models.LikedNews{
			UserID:  user.ID,
			NewsID:  news.ID,
			LikedAt: time.Now().Unix(),
		})
	} else if action == "undo" {
		err = database.DB.Model(&user).Association("LikedNews").Delete(&news)
	} else {
		c.Status(fiber.ErrBadRequest.Code)
		return c.JSON(fiber.Map{
			"message": "param action is invalid",
		})
	}

	if err != nil {
		c.Status(fiber.ErrBadRequest.Code)
		return c.JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	go func(user models.User) {
		if err := updateRecommendList(user); err != nil {
			log.Println(err)
		}
	}(user)

	return c.JSON(fiber.Map{
		"message": "success",
	})
}