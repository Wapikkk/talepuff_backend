package auth

import "github.com/gin-gonic/gin"

type Identity struct {
	UID   string
	Email string
}

const identityKey = "auth.identity"

func IdentityFrom(c *gin.Context) *Identity {
	value, _ := c.Get(identityKey)
	identity, _ := value.(*Identity)
	return identity
}
