// Copyright (c) 2014 The gomqtt Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package broker

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gomqtt/client"
	"github.com/gomqtt/packet"
	"github.com/gomqtt/tools"
	"github.com/gomqtt/transport"
	"github.com/stretchr/testify/assert"
)

func TestBroker(t *testing.T) {
	MandatoryAcceptanceTest(t, func(secure bool) *Broker {
		backend := NewMemoryBackend()

		broker := New()
		broker.Backend = backend

		if secure {
			backend.Logins = map[string]string{
				"allow": "allow",
			}
		}

		return broker
	})
}

func TestConnectTimeout(t *testing.T) {
	broker := New()
	broker.ConnectTimeout = 10 * time.Millisecond

	port, done := runBroker(t, broker, 1)

	conn, err := transport.Dial(port.URL())
	assert.NoError(t, err)

	pkt, err := conn.Receive()
	assert.Nil(t, pkt)
	assert.Error(t, err)

	<-done
}

func TestKeepAlive(t *testing.T) {
	t.Parallel()

	port, done := runBroker(t, New(), 1)

	opts := client.NewOptions()
	opts.KeepAlive = "1s"

	client := client.New()
	client.Callback = errorCallback(t)

	var reqCounter int32
	var respCounter int32

	client.Logger = func(message string) {
		if strings.Contains(message, "Pingreq") {
			atomic.AddInt32(&reqCounter, 1)
		} else if strings.Contains(message, "Pingresp") {
			atomic.AddInt32(&respCounter, 1)
		}
	}

	connectFuture, err := client.Connect(port.URL(), opts)
	assert.NoError(t, err)
	assert.NoError(t, connectFuture.Wait())
	assert.Equal(t, packet.ConnectionAccepted, connectFuture.ReturnCode)
	assert.False(t, connectFuture.SessionPresent)

	time.Sleep(2500 * time.Millisecond)

	err = client.Disconnect()
	assert.NoError(t, err)

	<-done

	assert.Equal(t, int32(2), atomic.LoadInt32(&reqCounter))
	assert.Equal(t, int32(2), atomic.LoadInt32(&respCounter))
}

func TestKeepAliveTimeout(t *testing.T) {
	t.Parallel()

	connect := packet.NewConnectPacket()
	connect.KeepAlive = 1

	connack := packet.NewConnackPacket()

	client := tools.NewFlow().
		Send(connect).
		Receive(connack).
		End()

	port, done := runBroker(t, New(), 1)

	conn, err := transport.Dial(port.URL())
	assert.NoError(t, err)
	assert.NotNil(t, conn)

	client.Test(t, conn)

	<-done
}

// TODO: Move Following Tests

func TestPublishResendQOS1(t *testing.T) {
	connect := packet.NewConnectPacket()
	connect.CleanSession = false
	connect.ClientID = "test"

	subscribe := packet.NewSubscribePacket()
	subscribe.PacketID = 1
	subscribe.Subscriptions = []packet.Subscription{
		{Topic: "test", QOS: 1},
	}

	publishOut := packet.NewPublishPacket()
	publishOut.PacketID = 2
	publishOut.Message.Topic = "test"
	publishOut.Message.QOS = 1

	publishIn := packet.NewPublishPacket()
	publishIn.PacketID = 1
	publishIn.Message.Topic = "test"
	publishIn.Message.QOS = 1

	pubackIn := packet.NewPubackPacket()
	pubackIn.PacketID = 1

	disconnect := packet.NewDisconnectPacket()

	port, done := runBroker(t, New(), 2)

	conn1, err := transport.Dial(port.URL())
	assert.NoError(t, err)
	assert.NotNil(t, conn1)

	tools.NewFlow().
		Send(connect).
		Skip(). // connack
		Send(subscribe).
		Skip(). // suback
		Send(publishOut).
		Skip(). // puback
		Receive(publishIn).
		Close().
		Test(t, conn1)

	conn2, err := transport.Dial(port.URL())
	assert.NoError(t, err)
	assert.NotNil(t, conn2)

	publishIn.Dup = true

	tools.NewFlow().
		Send(connect).
		Skip(). // connack
		Receive(publishIn).
		Send(pubackIn).
		Send(disconnect).
		Close().
		Test(t, conn2)

	<-done
}

func TestPubrelResendQOS2(t *testing.T) {
	connect := packet.NewConnectPacket()
	connect.CleanSession = false
	connect.ClientID = "test"

	subscribe := packet.NewSubscribePacket()
	subscribe.PacketID = 1
	subscribe.Subscriptions = []packet.Subscription{
		{Topic: "test", QOS: 2},
	}

	publishOut := packet.NewPublishPacket()
	publishOut.PacketID = 2
	publishOut.Message.Topic = "test"
	publishOut.Message.QOS = 2

	pubrelOut := packet.NewPubrelPacket()
	pubrelOut.PacketID = 2

	publishIn := packet.NewPublishPacket()
	publishIn.PacketID = 1
	publishIn.Message.Topic = "test"
	publishIn.Message.QOS = 2

	pubrecIn := packet.NewPubrecPacket()
	pubrecIn.PacketID = 1

	pubrelIn := packet.NewPubrelPacket()
	pubrelIn.PacketID = 1

	pubcompIn := packet.NewPubcompPacket()
	pubcompIn.PacketID = 1

	disconnect := packet.NewDisconnectPacket()

	port, done := runBroker(t, New(), 2)

	conn1, err := transport.Dial(port.URL())
	assert.NoError(t, err)
	assert.NotNil(t, conn1)

	tools.NewFlow().
		Send(connect).
		Skip(). // connack
		Send(subscribe).
		Skip(). // suback
		Send(publishOut).
		Skip(). // pubrec
		Send(pubrelOut).
		Skip(). // pubcomp
		Receive(publishIn).
		Send(pubrecIn).
		Close().
		Test(t, conn1)

	conn2, err := transport.Dial(port.URL())
	assert.NoError(t, err)
	assert.NotNil(t, conn2)

	publishIn.Dup = true

	tools.NewFlow().
		Send(connect).
		Skip(). // connack
		Receive(pubrelIn).
		Send(pubcompIn).
		Send(disconnect).
		Close().
		Test(t, conn2)

	<-done
}

func TestOfflineMessages(t *testing.T) {
	port, done := runBroker(t, New(), 3)

	options := client.NewOptions()
	options.CleanSession = false
	options.ClientID = "test"

	/* offline subscriber */

	client1 := client.New()
	client1.Callback = errorCallback(t)

	connectFuture1, err := client1.Connect(port.URL(), options)
	assert.NoError(t, err)
	assert.NoError(t, connectFuture1.Wait())
	assert.Equal(t, packet.ConnectionAccepted, connectFuture1.ReturnCode)
	assert.False(t, connectFuture1.SessionPresent)

	subscribeFuture, err := client1.Subscribe("test", 2)
	assert.NoError(t, err)
	assert.NoError(t, subscribeFuture.Wait())
	assert.Equal(t, []uint8{2}, subscribeFuture.ReturnCodes)

	err = client1.Disconnect()
	assert.NoError(t, err)

	/* publisher */

	client2 := client.New()
	client2.Callback = errorCallback(t)

	connectFuture2, err := client2.Connect(port.URL(), nil)
	assert.NoError(t, err)
	assert.NoError(t, connectFuture2.Wait())
	assert.Equal(t, packet.ConnectionAccepted, connectFuture2.ReturnCode)
	assert.False(t, connectFuture2.SessionPresent)

	publishFuture, err := client2.Publish("test", []byte("test"), 2, false)
	assert.NoError(t, err)
	assert.NoError(t, publishFuture.Wait())

	err = client2.Disconnect()
	assert.NoError(t, err)

	/* receiver */

	wait := make(chan struct{})

	client3 := client.New()
	client3.Callback = func(msg *packet.Message, err error) {
		assert.NoError(t, err)
		assert.Equal(t, "test", msg.Topic)
		assert.Equal(t, []byte("test"), msg.Payload)
		assert.Equal(t, uint8(2), msg.QOS)
		assert.False(t, msg.Retain)

		close(wait)
	}

	connectFuture3, err := client3.Connect(port.URL(), options)
	assert.NoError(t, err)
	assert.NoError(t, connectFuture3.Wait())
	assert.Equal(t, packet.ConnectionAccepted, connectFuture3.ReturnCode)
	assert.True(t, connectFuture3.SessionPresent)

	<-wait

	err = client3.Disconnect()
	assert.NoError(t, err)

	<-done
}
