package conf

var Conf AppConf

func SetAppConf(conf AppConf) {
	Conf = conf
}

type AppInfo struct {
	Port       int    `yaml:"port"`
	TLSKey     string `yaml:"tls-key"`
	TLSCert    string `yaml:"tls-cert"`
	TLSCaCerts string `yaml:"tls-ca-certs"`
}

type AppConf struct {
	AppInfo AppInfo `yaml:"vnc"`
}
