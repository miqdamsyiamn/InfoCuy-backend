package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// --- STRUCT DATA ---

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
	CreatedBy   string             `json:"created_by" bson:"created_by"` // Email pembuat
}

type User struct {
	ID       primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Email    string             `json:"email" bson:"email"`
	Password string             `json:"password" bson:"password"`
	Role     string             `json:"role" bson:"role"` // "admin" atau "user"
}

// Struct untuk input Login & Register
type AuthInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Struct untuk input Update Role (Admin Only)
type RoleInput struct {
	Role string `json:"role"`
}

var geoCollection *mongo.Collection
var userCollection *mongo.Collection

// --- KONEKSI DATABASE (VERSI AMAN & ROBUST) ---
func connectDB() {
	// 1. Ambil URI dari Environment Variable
	mongoURI := os.Getenv("MONGO_URI")
	
	// 2. Cek apakah kosong? (Penting agar tidak panic diam-diam)
	if mongoURI == "" {
		log.Fatal("❌ FATAL: MONGO_URI environment variable is not set! (Cek .env di local atau Environment Variables di Render)")
	}

	// 3. Konfigurasi Client
	clientOptions := options.Client().ApplyURI(mongoURI)
	
	// 4. Coba Buat Koneksi
	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		log.Fatal("❌ Gagal membuat client MongoDB:", err)
	}

	// 5. Cek Koneksi (Ping) - Memastikan internet/kredensial benar
	err = client.Ping(context.TODO(), nil)
	if err != nil {
		log.Fatal("❌ Gagal ping ke MongoDB (Cek koneksi internet atau password):", err)
	}

	fmt.Println("✅ Terhubung sukses ke MongoDB Atlas!")
	geoCollection = client.Database("geo_db").Collection("geo_data")
	userCollection = client.Database("geo_db").Collection("user")
}

func main() {
	// --- LOAD ENV FILE (Hanya untuk Localhost) ---
	// Di Render, file .env tidak ada, jadi ini akan error tapi kita abaikan (itu normal)
	err := godotenv.Load()
	if err != nil {
		fmt.Println("Info: Tidak ada file .env (Menggunakan System Environment Variables)")
	}

	// Hubungkan Database
	connectDB()

	r := gin.Default()

	// --- SETUP CORS ---
	// Agar Frontend (Vercel/Localhost) bisa akses Backend ini
	config := cors.DefaultConfig()
	config.AllowAllOrigins = true // Bolehkan semua domain (untuk tahap awal deploy)
	config.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "X-User-Email"}
	r.Use(cors.New(config))

	// ==========================================
	// 1. AUTHENTICATION ROUTES
	// ==========================================

	// REGISTER
	r.POST("/register", func(c *gin.Context) {
		var input AuthInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		var existingUser User
		err := userCollection.FindOne(context.TODO(), bson.M{"email": input.Email}).Decode(&existingUser)
		if err == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Email sudah terdaftar!"})
			return
		}

		newUser := User{
			ID:       primitive.NewObjectID(),
			Email:    input.Email,
			Password: input.Password, // Di production sebaiknya di-hash
			Role:     "user",         
		}

		_, err = userCollection.InsertOne(context.TODO(), newUser)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal registrasi"})
			return
		}

		c.JSON(http.StatusCreated, gin.H{"message": "Registrasi berhasil!", "data": newUser})
	})

	// LOGIN
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

		c.JSON(http.StatusOK, gin.H{
			"message": "Login sukses",
			"user":    user,
		})
	})

	// ==========================================
	// 2. LOCATION ROUTES (CRUD DATA PETA)
	// ==========================================

	// GET ALL LOCATIONS
	r.GET("/locations", func(c *gin.Context) {
		var locations []Location
		cursor, err := geoCollection.Find(context.TODO(), bson.M{})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer cursor.Close(context.TODO())

		for cursor.Next(context.TODO()) {
			var loc Location
			cursor.Decode(&loc)
			locations = append(locations, loc)
		}
		
		// Pastikan return array kosong [] bukan null jika data kosong
		if locations == nil {
			locations = []Location{}
		}
		c.JSON(http.StatusOK, locations)
	})

	// ADD LOCATION
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

		_, err := geoCollection.InsertOne(context.TODO(), newLocation)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal simpan data"})
			return
		}

		c.JSON(http.StatusCreated, gin.H{"message": "Lokasi ditambahkan!", "data": newLocation})
	})

	// EDIT LOCATION
	r.PUT("/locations/:id", func(c *gin.Context) {
		idParam := c.Param("id")
		objID, _ := primitive.ObjectIDFromHex(idParam)
		
		requestorEmail := c.GetHeader("X-User-Email")
		var requestor User
		err := userCollection.FindOne(context.TODO(), bson.M{"email": requestorEmail}).Decode(&requestor)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User tidak dikenali"})
			return
		}

		var existingLoc Location
		err = geoCollection.FindOne(context.TODO(), bson.M{"_id": objID}).Decode(&existingLoc)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Lokasi tidak ditemukan"})
			return
		}

		// Validasi Hak Akses (Admin ATAU Pemilik Data)
		if requestor.Role != "admin" && existingLoc.CreatedBy != requestor.Email {
			c.JSON(http.StatusForbidden, gin.H{"error": "Anda tidak berhak mengedit data orang lain!"})
			return
		}

		var updateData Location
		if err := c.ShouldBindJSON(&updateData); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		update := bson.M{
			"$set": bson.M{
				"name":        updateData.Name,
				"category":    updateData.Category,
				"coordinates": updateData.Coordinates,
				"address":     updateData.Address,
			},
		}

		_, err = geoCollection.UpdateOne(context.TODO(), bson.M{"_id": objID}, update)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mengupdate data"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Data berhasil diupdate"})
	})

	// DELETE LOCATION
	r.DELETE("/locations/:id", func(c *gin.Context) {
		idParam := c.Param("id")
		objID, _ := primitive.ObjectIDFromHex(idParam)

		requestorEmail := c.GetHeader("X-User-Email")
		var requestor User
		err := userCollection.FindOne(context.TODO(), bson.M{"email": requestorEmail}).Decode(&requestor)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User tidak dikenali"})
			return
		}

		var existingLoc Location
		err = geoCollection.FindOne(context.TODO(), bson.M{"_id": objID}).Decode(&existingLoc)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Lokasi tidak ditemukan"})
			return
		}

		if requestor.Role != "admin" && existingLoc.CreatedBy != requestor.Email {
			c.JSON(http.StatusForbidden, gin.H{"error": "Anda tidak berhak menghapus data orang lain!"})
			return
		}

		_, err = geoCollection.DeleteOne(context.TODO(), bson.M{"_id": objID})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal menghapus data"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Data berhasil dihapus"})
	})

	// ==========================================
	// 3. ADMIN USER MANAGEMENT
	// ==========================================

	funcIsAdmin := func(email string) bool {
		var u User
		userCollection.FindOne(context.TODO(), bson.M{"email": email}).Decode(&u)
		return u.Role == "admin"
	}

	// GET ALL USERS
	r.GET("/users", func(c *gin.Context) {
		requestorEmail := c.GetHeader("X-User-Email")
		if !funcIsAdmin(requestorEmail) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Khusus Admin!"})
			return
		}

		var users []User
		cursor, _ := userCollection.Find(context.TODO(), bson.M{})
		defer cursor.Close(context.TODO())
		for cursor.Next(context.TODO()) {
			var u User
			cursor.Decode(&u)
			users = append(users, u)
		}
		if users == nil { users = []User{} }
		c.JSON(http.StatusOK, users)
	})

	// UPDATE USER ROLE
	r.PUT("/users/:id/role", func(c *gin.Context) {
		requestorEmail := c.GetHeader("X-User-Email")
		if !funcIsAdmin(requestorEmail) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Khusus Admin!"})
			return
		}

		idParam := c.Param("id")
		objID, _ := primitive.ObjectIDFromHex(idParam)
		var input RoleInput
		c.ShouldBindJSON(&input)

		_, err := userCollection.UpdateOne(context.TODO(), bson.M{"_id": objID}, bson.M{"$set": bson.M{"role": input.Role}})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal update role"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Role user berhasil diubah"})
	})

	// DELETE USER
	r.DELETE("/users/:id", func(c *gin.Context) {
		requestorEmail := c.GetHeader("X-User-Email")
		if !funcIsAdmin(requestorEmail) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Khusus Admin!"})
			return
		}

		idParam := c.Param("id")
		objID, _ := primitive.ObjectIDFromHex(idParam)

		_, err := userCollection.DeleteOne(context.TODO(), bson.M{"_id": objID})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal hapus user"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "User berhasil dihapus"})
	})

	// --- SETUP PORT UNTUK RENDER ---
	// Render akan otomatis inject environment variable bernama PORT
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Fallback ke 8080 jika dijalankan di localhost
	}

	fmt.Println("🚀 Server berjalan di port " + port)
	r.Run(":" + port)
}