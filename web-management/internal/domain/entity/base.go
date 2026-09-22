package entity

import "time"

// Base contains common fields for all entities.
// Embed this in your entity structs.
type Base struct {
	ID        string    `json:"id" gorm:"primaryKey;type:varchar(36);default:gen_random_uuid()"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// SampleEntity is a SAMPLE entity — replace with your own.
// Delete this when building your actual service.
type SampleEntity struct {
	Base
	Name        string `json:"name" gorm:"type:varchar(255);not null"`
	Description string `json:"description" gorm:"type:text"`
	Status      string `json:"status" gorm:"type:varchar(20);default:'active'"`
}

func (SampleEntity) TableName() string { return "sample_entities" }

/*
=== HOW TO ADD YOUR OWN ENTITY ===

1. Create your entity in this package (e.g., employee.go):

    type Employee struct {
        Base
        FirstName string `json:"first_name" gorm:"type:varchar(100);not null"`
        LastName  string `json:"last_name" gorm:"type:varchar(100)"`
        Email     string `json:"email" gorm:"type:varchar(255);uniqueIndex;not null"`
        Phone     string `json:"phone" gorm:"type:varchar(20)"`
        Status    string `json:"status" gorm:"type:varchar(20);default:'active'"`
    }

    func (Employee) TableName() string { return "employees" }

2. Add AutoMigrate in main.go:
    database.AutoMigrate(db, &entity.Employee{})

3. Create DTOs in dto/request/ and dto/response/

4. Create mapper in dto/mapper/

5. Create repository interface in domain/service/

6. Create repository implementation in adapter/persistence/postgres/

7. Create handler in adapter/api/http/handler/

8. Add routes in router.go
*/
