package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
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

// --- KONEKSI DATABASE ---
func connectDB() {
	// Ganti string koneksi jika perlu
	clientOptions := options.Client().ApplyURI("mongodb+srv://Maiys0311_db:Miqdam031104@maiys0311.w9twyze.mongodb.net/")
	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("✅ Terhubung ke MongoDB!")
	geoCollection = client.Database("geo_db").Collection("geo_data")
	userCollection = client.Database("geo_db").Collection("user")
}

func main() {
	connectDB()

	r := gin.Default()

	// Setup CORS: Izinkan Header 'X-User-Email' untuk identifikasi user
	config := cors.DefaultConfig()
	config.AllowAllOrigins = true
	config.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "X-User-Email"}
	r.Use(cors.New(config))

	// ==========================================
	// 1. AUTHENTICATION ROUTES
	// ==========================================

	// REGISTER (Otomatis jadi USER)
	r.POST("/register", func(c *gin.Context) {
		var input AuthInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Cek apakah email sudah ada
		var existingUser User
		err := userCollection.FindOne(context.TODO(), bson.M{"email": input.Email}).Decode(&existingUser)
		if err == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Email sudah terdaftar!"})
			return
		}

		// Buat User Baru (Paksa Role jadi 'user')
		newUser := User{
			ID:       primitive.NewObjectID(),
			Email:    input.Email,
			Password: input.Password, // Idealnya di-hash dulu
			Role:     "user",         // Default role
		}

		_, err = userCollection.InsertOne(context.TODO(), newUser)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal registrasi"})
			return
		}

		c.JSON(http.StatusCreated, gin.H{"message": "Registrasi berhasil!", "data": newUser})
	})

	// LOGIN (Cek Email & Password)
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
			"user":    user, // Frontend akan menyimpan data ini (terutama role & email)
		})
	})

	// ==========================================
	// 2. LOCATION ROUTES (CRUD DATA PETA)
	// ==========================================

	// GET ALL LOCATIONS (Public - Semua bisa lihat)
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
		c.JSON(http.StatusOK, locations)
	})

	// ADD LOCATION (Butuh Identitas User)
	r.POST("/locations", func(c *gin.Context) {
		// Ambil siapa yang request dari Header (Dikirim oleh Frontend)
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
		newLocation.CreatedBy = userEmail // Otomatis isi pembuatnya

		_, err := geoCollection.InsertOne(context.TODO(), newLocation)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal simpan data"})
			return
		}

		c.JSON(http.StatusCreated, gin.H{"message": "Lokasi ditambahkan!", "data": newLocation})
	})

	// EDIT LOCATION (Logic: Admin OR Owner)
	r.PUT("/locations/:id", func(c *gin.Context) {
		idParam := c.Param("id")
		objID, _ := primitive.ObjectIDFromHex(idParam)
		
		// 1. Ambil Data User yang Request
		requestorEmail := c.GetHeader("X-User-Email")
		var requestor User
		err := userCollection.FindOne(context.TODO(), bson.M{"email": requestorEmail}).Decode(&requestor)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User tidak dikenali/login"})
			return
		}

		// 2. Ambil Data Lokasi yang mau diedit (Cek punya siapa)
		var existingLoc Location
		err = geoCollection.FindOne(context.TODO(), bson.M{"_id": objID}).Decode(&existingLoc)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Lokasi tidak ditemukan"})
			return
		}

		// 3. LOGIKA SAKTI: Cek Hak Akses
		// Boleh edit JIKA: Role Admin ATAU Email Pembuat == Email Requestor
		if requestor.Role != "admin" && existingLoc.CreatedBy != requestor.Email {
			c.JSON(http.StatusForbidden, gin.H{"error": "Anda tidak berhak mengedit data orang lain!"})
			return
		}

		// 4. Proses Update
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
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal update data"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Data berhasil diupdate"})
	})

	// DELETE LOCATION (Logic: Admin OR Owner)
	r.DELETE("/locations/:id", func(c *gin.Context) {
		idParam := c.Param("id")
		objID, _ := primitive.ObjectIDFromHex(idParam)

		// 1. Cek User
		requestorEmail := c.GetHeader("X-User-Email")
		var requestor User
		err := userCollection.FindOne(context.TODO(), bson.M{"email": requestorEmail}).Decode(&requestor)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User tidak dikenali"})
			return
		}

		// 2. Cek Lokasi
		var existingLoc Location
		err = geoCollection.FindOne(context.TODO(), bson.M{"_id": objID}).Decode(&existingLoc)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Lokasi tidak ditemukan"})
			return
		}

		// 3. Cek Hak Akses
		if requestor.Role != "admin" && existingLoc.CreatedBy != requestor.Email {
			c.JSON(http.StatusForbidden, gin.H{"error": "Anda tidak berhak menghapus data orang lain!"})
			return
		}

		// 4. Hapus
		_, err = geoCollection.DeleteOne(context.TODO(), bson.M{"_id": objID})
		c.JSON(http.StatusOK, gin.H{"message": "Data berhasil dihapus"})
	})

	// ==========================================
	// 3. ADMIN USER MANAGEMENT (ADMIN ONLY)
	// ==========================================

	// Helper function untuk cek admin
	funcIsAdmin := func(email string) bool {
		var u User
		userCollection.FindOne(context.TODO(), bson.M{"email": email}).Decode(&u)
		return u.Role == "admin"
	}

	// GET ALL USERS (Admin Only)
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
		c.JSON(http.StatusOK, users)
	})

	// UPDATE USER ROLE (Admin Only - Promote/Demote)
	r.PUT("/users/:id/role", func(c *gin.Context) {
		requestorEmail := c.GetHeader("X-User-Email")
		if !funcIsAdmin(requestorEmail) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Khusus Admin!"})
			return
		}

		idParam := c.Param("id")
		objID, _ := primitive.ObjectIDFromHex(idParam)
		var input RoleInput
		c.ShouldBindJSON(&input) // misal: {"role": "admin"}

		_, err := userCollection.UpdateOne(context.TODO(), bson.M{"_id": objID}, bson.M{"$set": bson.M{"role": input.Role}})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal update role"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Role user berhasil diubah"})
	})

	// DELETE USER (Admin Only)
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

	r.Run(":8080")
}