package main
import (
	"fmt"
	"log"
	"golang.org/x/crypto/bcrypt"
)
func main() {
	password := "q2e4t6u8"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("原始密码: %s\n", password)
	fmt.Printf("bcrypt 哈希: %s\n", string(hash))
	err = bcrypt.CompareHashAndPassword(hash, []byte(password))
	if err == nil {
		fmt.Println("✅ 验证成功！")
	} else {
		fmt.Println("❌ 验证失败！")
	}
}
