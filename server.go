package natshelper

import (
	"errors"
	natsgo "github.com/nats-io/nats.go"
	"go.uber.org/zap"
	"time"
)

type natsState int

const (
	sConnected natsState = iota
	sError
	sNew
	sClosed
	sStopped
)

var (
	// ErrDuplicateProcessor - processor already exists
	ErrDuplicateProcessor = errors.New("processor is duplicate")
	ErrNotConnected       = errors.New("nats: not connected")
)

type processorRecord struct {
	Processor
	RequestProcessor
	subscription *natsgo.Subscription
	subject      string
	name         string
}

// Server wrapper around NATS server
type Server struct {
	client     *natsgo.Conn
	state      natsState
	log        *zap.Logger
	stopCh     chan struct{}
	processors map[string]processorRecord
	config     map[string]interface{}
}

// Processor is interface for processor of incoming messages
type Processor interface {
	// ProcessNatsMessage is called on incoming message
	ProcessNatsMessage(subject string, msg []byte) error
}

// RequestProcessor is interface for processor of incoming requests
type RequestProcessor interface {
	ProcessNatsRequest(topic string, request []byte) (response []byte, err error)
}

// NewServer inits the nats with given configuration and tries to connect
func NewServer(cfg map[string]interface{}) (*Server, error) {
	srv := &Server{
		state:      sNew,
		processors: map[string]processorRecord{},
		config:     cfg,
	}

	err := srv.connect()
	return srv, err
}

func (ns *Server) connect() error {
	o := &natsgo.Options{}
	o.AllowReconnect = true
	o.MaxReconnect = 60
	o.ReconnectWait = time.Second
	o.Url, _ = ns.config["url"].(string)
	if o.Url == "" {
		o.Url = natsgo.DefaultURL
	}

	var err error
	ns.client, err = o.Connect()
	if err != nil {
		return err
	}
	ns.client.SetClosedHandler(
		func(conn *natsgo.Conn) {
			ns.log.Info("nats connection was closed")
			if ns.state != sStopped {
				ns.state = sClosed
				go func() {
					<-time.NewTimer(5 * time.Second).C
					ns.log.Info("trying to reconnect")
					ns.connect()
				}()
			}
		},
	)
	ns.client.SetErrorHandler(
		func(conn *natsgo.Conn, s *natsgo.Subscription, err error) {
			ns.state = sError
			if ns.log != nil {
				ns.log.Error("connect: error while connecting", zap.Error(err))
			}
		},
	)
	ns.client.SetReconnectHandler(
		func(conn *natsgo.Conn) {
			ns.state = sConnected
			if ns.log != nil {
				ns.log.Debug("connect: reconnected")
			}
		},
	)
	if ns.log != nil {
		ns.log.Debug("connect: nats client connection request was sent successfully")
	}
	ns.state = sConnected
	return ns.listen()
}

// Stop stops the server
func (ns *Server) Stop() {
	if ns.stopCh != nil {
		close(ns.stopCh)
	}
	ns.state = sStopped
	ns.client.Close()
}

// AddProcessor adds the incoming messages processor
func (ns *Server) AddProcessor(name string, subject string, processor Processor, replace bool) error {
	_, ok := ns.processors[name]
	if ok && !replace {
		return ErrDuplicateProcessor
	}
	pr := processorRecord{
		Processor: processor,
		subject:   subject,
		name:      name,
	}
	var err error
	if ns.state == sConnected {
		err = ns.subscribeProcessor(&pr)
	}
	ns.processors[name] = pr
	return err
}

// AddRequestProcessor adds the incoming request processor
func (ns *Server) AddRequestProcessor(name string, subject string, processor RequestProcessor, replace bool) error {
	_, ok := ns.processors[name]
	if ok && !replace {
		return ErrDuplicateProcessor
	}
	pr := processorRecord{
		RequestProcessor: processor,
		subject:          subject,
		name:             name,
	}
	var err error
	if ns.state == sConnected {
		err = ns.subscribeProcessor(&pr)
	}
	ns.processors[name] = pr
	return err
}

func (ns *Server) listen() (err error) {
	for name, pr := range ns.processors {
		err = ns.subscribeProcessor(&pr)
		ns.processors[name] = pr
	}
	return
}

func (ns *Server) subscribeProcessor(pr *processorRecord) error {
	var err error
	pr.subscription, err = ns.client.Subscribe(
		pr.subject,
		func(msg *natsgo.Msg) {
			ns.processMessage(msg, pr)
		},
	)
	return err
}

func (ns *Server) processMessage(msg *natsgo.Msg, pr *processorRecord) {
	defer func() {
		if problem := recover(); problem != nil {
			if ns.log != nil {
				ns.log.Error(
					"recovered while processing a message",
					zap.String("processor", pr.name),
					zap.String("subject", msg.Subject),
					zap.Any("problem", problem),
					zap.Stack("stack"),
				)
			}
		}
	}()
	if pr.Processor != nil {
		_ = pr.Processor.ProcessNatsMessage(msg.Subject, msg.Data)
	} else if pr.RequestProcessor != nil {
		resp, err := pr.RequestProcessor.ProcessNatsRequest(msg.Subject, msg.Data)
		if err != nil {
			if ns.log != nil {
				ns.log.Error("while processing message", zap.String("processor", pr.name), zap.Error(err))
			}
			response := natsgo.Msg{
				Subject: msg.Reply,
				Header:  natsgo.Header{"state": []string{"error"}},
			}
			err = ns.client.PublishMsg(&response)
			if err != nil {
				if ns.log != nil {
					ns.log.Error("while publishing error response", zap.String("processor", pr.name), zap.Error(err))
				}
			}
		} else {
			err = ns.client.Publish(msg.Reply, resp)
			if err != nil {
				if ns.log != nil {
					ns.log.Error("while publishing response", zap.String("processor", pr.name), zap.Error(err))
				}
			}
		}
	}
}

// Publish allows to publish a message on given topic
func (ns *Server) Publish(subject string, kind Kind, message []byte, options ...Option) error {
	if kind != KindUnknown {
		subject = string(kind) + "." + subject
	}

	if ns.state != sClosed && ns.state != sStopped {
		var err error
		err = ns.client.Publish(subject, message)
		if err != nil {
			if ns.log != nil {
				ns.log.Error("when publishing message", zap.Error(err))
			}
		} else {
			if ns.log != nil {
				ns.log.Debug("published successfully", zap.Any("msg", message), zap.String("subject", subject))
			}
		}
		return err
	}
	if ns.log != nil {
		ns.log.Error("publish was called on not connected server: ", zap.Int("state", int(ns.state)))
	}
	return ErrNotConnected

}

func (ns *Server) RequestSync(
	subject string,
	message []byte,
	timeout time.Duration,
	options ...Option,
) (resp []byte, err error) {
	subject = string(KindRequest) + "." + subject
	respMsg, err := ns.client.Request(subject, message, timeout)
	if respMsg != nil {
		resp = respMsg.Data
	}
	return
}
