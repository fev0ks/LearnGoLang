package main

//REDIS_HOST=127.0.0.1;
//REDIS_PASSWORD=password;
//REDIS_PORT=6379;
//REDIS_USER=default;

type RedisConfig struct {
	Host     string `env:"REDIS_HOST"`
	Port     string `env:"REDIS_PORT"`
	User     string `env:"REDIS_USER"`
	Password string `env:"REDIS_PASSWORD"`
	DB       int    `env:"REDIS_PASSKEY_DB,default=0"`
}

func getConfig() RedisConfig {
	return RedisConfig{
		Host:     "127.0.0.1",
		Port:     "6379",
		User:     "default",
		Password: "password",
	}
}
