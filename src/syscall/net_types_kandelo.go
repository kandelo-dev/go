package syscall

const (
	AF_UNSPEC = 0
	AF_UNIX   = 1
	AF_INET   = 2
	AF_INET6  = 10
)

const (
	SOCK_STREAM    = 1
	SOCK_DGRAM     = 2
	SOCK_RAW       = 3
	SOCK_SEQPACKET = 5
	SOCK_NONBLOCK  = 0o4000
	SOCK_CLOEXEC   = 0o2000000
)

const (
	IPPROTO_IP   = 0
	IPPROTO_IPV4 = 4
	IPPROTO_TCP  = 6
	IPPROTO_UDP  = 17
	IPPROTO_IPV6 = 41
)

const (
	SOMAXCONN       = 128
	SOL_SOCKET      = 1
	SO_ERROR        = 4
	SO_REUSEADDR    = 2
	SO_TYPE         = 3
	SO_BROADCAST    = 6
	SO_SNDBUF       = 7
	SO_RCVBUF       = 8
	SO_KEEPALIVE    = 9
	SO_LINGER       = 13
	TCP_NODELAY     = 1
	TCP_KEEPIDLE    = 4
	TCP_KEEPINTVL   = 5
	TCP_KEEPCNT     = 6
	IPV6_V6ONLY     = 26
	F_DUPFD_CLOEXEC = 1030
	SYS_FCNTL       = 10
)

type Sockaddr any

type SockaddrInet4 struct {
	Port int
	Addr [4]byte
}

type SockaddrInet6 struct {
	Port   int
	ZoneId uint32
	Addr   [16]byte
}

type SockaddrUnix struct {
	Name string
}

type Linger struct {
	Onoff  int32
	Linger int32
}
