package handler

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// --- SEMUA STRUCT DATA ---
type Coordinates struct {
	Lat float64 `json:"lat" bson:"lat"`
	Lng float64 `json:"lng" bson:"lng"`
}
type Location struct {
	ID          primitive.ObjectID `json:"_id,omitempty" bson:"_id,omitempty"`
	Name        string             `json:"name" bson:"name"`
	Category    string             `json:"category" bson:"category"`
	Coordinates Coordinates        `json:"coordinates" bson:"coordinates"`
	Address     string             `json:"address" bson:"address"`
	CreatedBy   string             `json:"created_by" bson:"created_by"`
}
type User struct {
	ID       primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Email    string             `json:"email" bson:"email"`
	Password string             `json:"password" bson:"password"`
	Role     string             `json:"role" bson:"role"`
}
type AuthInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
type RoleInput struct {
	Role string `json:"role"`
}

// Global Variables
var (
	app           *gin.Engine
	geoCollection *mongo.Collection
	userCollection *mongo.Collection
	once          sync.Once // Agar init hanya jalan sekali
)

// --- KONEKSI DB ---
func connectDB() {
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		log.Println("Warning: MONGO_URI is missing")
		return
	}
	clientOptions := options.Client().ApplyURI(mongoURI)
	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		log.Fatal(err)
	}
	err = client.Ping(context.TODO(), nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("✅ Connected to MongoDB!")
	geoCollection = client.Database("geo_db").Collection("geo_data")
	userCollection = client.Database("geo_db").Collection("user")
}

// --- SETUP ROUTER (EXPORTED agar bisa dipanggil main.go) ---
func SetupRouter() *gin.Engine {
	// Gunakan sync.Once agar DB tidak connect berkali-kali saat di Vercel
	once.Do(func() {
		connectDB()
		r := gin.New()
		r.Use(gin.Logger())
		r.Use(gin.Recovery())

		config := cors.DefaultConfig()
		config.AllowAllOrigins = true
		config.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "X-User-Email"}
		r.Use(cors.New(config))

		// === DEFINISI ROUTES ===
		
		// 1. REGISTER
		r.POST("/register", func(c *gin.Context) {
			var input AuthInput
			if err := c.ShouldBindJSON(&input); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			var existingUser User
			userCollection.FindOne(context.TODO(), bson.M{"email": input.Email}).Decode(&existingUser)
			if existingUser.Email != "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Email sudah terdaftar!"})
				return
			}
			newUser := User{ID: primitive.NewObjectID(), Email: input.Email, Password: input.Password, Role: "user"}
			userCollection.InsertOne(context.TODO(), newUser)
			c.JSON(http.StatusCreated, gin.H{"message": "Registrasi berhasil!", "data": newUser})
		})

		// 2. LOGIN
		r.POST("/login", func(c *gin.Context) {
			var input AuthInput
			if err := c.ShouldBindJSON(&input); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			var user User
			err := userCollection.FindOne(context.TODO(), bson.M{"email": input.Email, "password": input.Password}).Decode(&user)
			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Email atau Password salah"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"message": "Login sukses", "user": user})
		})

		// 3. GET LOCATIONS
		r.GET("/locations", func(c *gin.Context) {
			var locations []Location
			cursor, _ := geoCollection.Find(context.TODO(), bson.M{})
			defer cursor.Close(context.TODO())
			for cursor.Next(context.TODO()) {
				var loc Location
				cursor.Decode(&loc)
				locations = append(locations, loc)
			}
			if locations == nil { locations = []Location{} }
			c.JSON(http.StatusOK, locations)
		})

		// 4. ADD LOCATION
		r.POST("/locations", func(c *gin.Context) {
			userEmail := c.GetHeader("X-User-Email")
			if userEmail == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Anda harus login!"})
				return
			}
			var newLocation Location
			if err := c.ShouldBindJSON(&newLocation); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			newLocation.ID = primitive.NewObjectID()
			newLocation.CreatedBy = userEmail
			geoCollection.InsertOne(context.TODO(), newLocation)
			c.JSON(http.StatusCreated, gin.H{"message": "Lokasi ditambahkan!", "data": newLocation})
		})

		// 5. EDIT LOCATION
		r.PUT("/locations/:id", func(c *gin.Context) {
			idParam := c.Param("id")
			objID, _ := primitive.ObjectIDFromHex(idParam)
			requestorEmail := c.GetHeader("X-User-Email")
			
			var requestor User
			userCollection.FindOne(context.TODO(), bson.M{"email": requestorEmail}).Decode(&requestor)
			var existingLoc Location
			geoCollection.FindOne(context.TODO(), bson.M{"_id": objID}).Decode(&existingLoc)

			if requestor.Role != "admin" && existingLoc.CreatedBy != requestor.Email {
				c.JSON(http.StatusForbidden, gin.H{"error": "Akses ditolak"})
				return
			}

			var updateData Location
			c.ShouldBindJSON(&updateData)
			update := bson.M{
				"$set": bson.M{
					"name": updateData.Name, "category": updateData.Category,
					"coordinates": updateData.Coordinates, "address": updateData.Address,
				},
			}
			geoCollection.UpdateOne(context.TODO(), bson.M{"_id": objID}, update)
			c.JSON(http.StatusOK, gin.H{"message": "Data diupdate"})
		})

		// 6. DELETE LOCATION
		r.DELETE("/locations/:id", func(c *gin.Context) {
			idParam := c.Param("id")
			objID, _ := primitive.ObjectIDFromHex(idParam)
			requestorEmail := c.GetHeader("X-User-Email")
			
			var requestor User
			userCollection.FindOne(context.TODO(), bson.M{"email": requestorEmail}).Decode(&requestor)
			var existingLoc Location
			geoCollection.FindOne(context.TODO(), bson.M{"_id": objID}).Decode(&existingLoc)

			if requestor.Role != "admin" && existingLoc.CreatedBy != requestor.Email {
				c.JSON(http.StatusForbidden, gin.H{"error": "Akses ditolak"})
				return
			}
			geoCollection.DeleteOne(context.TODO(), bson.M{"_id": objID})
			c.JSON(http.StatusOK, gin.H{"message": "Data dihapus"})
		})

		// 7. GET USERS (Admin)
		r.GET("/users", func(c *gin.Context) {
			requestorEmail := c.GetHeader("X-User-Email")
			var u User
			userCollection.FindOne(context.TODO(), bson.M{"email": requestorEmail}).Decode(&u)
			if u.Role != "admin" {
				c.JSON(http.StatusForbidden, gin.H{"error": "Khusus Admin"})
				return
			}
			var users []User
			cursor, _ := userCollection.Find(context.TODO(), bson.M{})
			defer cursor.Close(context.TODO())
			for cursor.Next(context.TODO()) {
				var usr User
				cursor.Decode(&usr)
				users = append(users, usr)
			}
			if users == nil { users = []User{} }
			c.JSON(http.StatusOK, users)
		})

		// 8. UPDATE USER ROLE
		r.PUT("/users/:id/role", func(c *gin.Context) {
			requestorEmail := c.GetHeader("X-User-Email")
			var u User
			userCollection.FindOne(context.TODO(), bson.M{"email": requestorEmail}).Decode(&u)
			if u.Role != "admin" {
				c.JSON(http.StatusForbidden, gin.H{"error": "Khusus Admin"})
				return
			}
			idParam := c.Param("id")
			objID, _ := primitive.ObjectIDFromHex(idParam)
			var input RoleInput
			c.ShouldBindJSON(&input)
			userCollection.UpdateOne(context.TODO(), bson.M{"_id": objID}, bson.M{"$set": bson.M{"role": input.Role}})
			c.JSON(http.StatusOK, gin.H{"message": "Role diubah"})
		})

		// 9. DELETE USER
		r.DELETE("/users/:id", func(c *gin.Context) {
			requestorEmail := c.GetHeader("X-User-Email")
			var u User
			userCollection.FindOne(context.TODO(), bson.M{"email": requestorEmail}).Decode(&u)
			if u.Role != "admin" {
				c.JSON(http.StatusForbidden, gin.H{"error": "Khusus Admin"})
				return
			}
			idParam := c.Param("id")
			objID, _ := primitive.ObjectIDFromHex(idParam)
			userCollection.DeleteOne(context.TODO(), bson.M{"_id": objID})
			c.JSON(http.StatusOK, gin.H{"message": "User dihapus"})
		})

		app = r
	})
	return app
}

// --- ENTRY POINT VERCEL ---
// Fungsi ini yang dicari oleh Vercel
func Handler(w http.ResponseWriter, r *http.Request) {
	// Pastikan Router sudah siap
	router := SetupRouter()
	// Jalankan request
	router.ServeHTTP(w, r)
}