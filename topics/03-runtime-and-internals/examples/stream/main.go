package main

import (
	"context"
	"git.dnkbit.one/dnkbit/backend/proto-files/go/v1/common"
	pb "git.dnkbit.one/dnkbit/backend/proto-files/go/v1/external_api/stream_service_external" // Импортируй сгенерированный gRPC-код
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"log"
	"time"
)

func main() {
	// Подключаемся к gRPC-серверу
	conn, err := grpc.NewClient("localhost:8076", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{Time: time.Duration(100) * time.Second, Timeout: 0}),
	)
	if err != nil {
		log.Fatalf("Не удалось подключиться: %v", err)
	}
	defer conn.Close()

	client := pb.NewStreamServiceClient(conn) // Создаем gRPC-клиента

	// Открываем поток
	md := metadata.Pairs("x-device-id", "1", "x-locale", "en", "x-platform-type", "3", "authorization", "eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9.eyJkZXZpY2VJZCI6IjEiLCJlbWFpbCI6IiIsImV4cCI6MTc5NDQzMzA0Mywib3BlcmF0aW9uVHlwZSI6IiIsInBob25lIjoiKzc5NjU3Nzc3Nzc4IiwicGxhdGZvcm0iOiIiLCJ0b2tlblR5cGUiOiJBQ0NFU1MiLCJ1c2VySWQiOiI0ZjUwZTA4OC0xYzNhLTQ3MWUtYTgwNC0wOTM0NDk1NDkwMGEiLCJ1c2VyVHlwZSI6IiJ9.X6aCMwItQm3Z8_Lz0pFm9-9IZk5Z-jqUGViAoxIp9ecKKmF1A_xrng6q2SDZVb7WbyT5ZhcFxaMdd1dL-8U4JA")
	ctx := metadata.NewOutgoingContext(context.Background(), md)
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	// Функция пересоздания stream
	createStream := func() (pb.StreamService_TrackUserEventsClient, error) {
		stream, err := client.TrackUserEvents(ctx, &common.EmptyRequest{})
		if err != nil {
			log.Printf("Ошибка при создании stream: %v", err)
			return nil, err
		}
		log.Println("Stream успешно создан")
		return stream, nil
	}

	// Инициализация stream
	stream, err := createStream()
	if err != nil {
		return
	}

	// Запускаем горутину для отправки ping каждые 5 секунд
	go func() {
		for {
			if stream == nil {
				log.Println("stream is nil")
				break
			}

			select {
			case <-stream.Context().Done():
				log.Println("context done")
			default:
				err := stream.SendMsg(&common.UserEvent{}) // Отправляем пустой запрос
				if err != nil {
					log.Printf("Ошибка отправки ping: %v", err)
					//return
				}
			}

			log.Println("Ping отправлен")
			time.Sleep(5 * time.Second) // Ждем 5 секунд перед отправкой
		}
	}()

	// Получаем ответы (Pong)
	go func() {
		for {
			resp, err := stream.Recv()
			if err != nil {
				st, _ := status.FromError(err)
				log.Printf("Ошибка получения ответа: %v", st.Message())
				break
			}
			log.Printf("Получен Pong: %s", resp.String())
		}
	}()

	select {}
}

//
//func (server *GRPCServer) broadcastEventToUsersNew(ctx context.Context,
//	wait *sync.WaitGroup,
//	event *common.UserEvent,
//	senderId string,
//	agentIds []uuid.UUID,
//) {
//	for _, agentId := range agentIds {
//		if senderId != agentId.String() {
//			agentStream, agentExists := server.Cache.GetUserStream(agentId.String())
//			if agentExists {
//				wait.Add(1)
//				select {
//				case <-agentStream.Stream.Context().Done():
//					wait.Done()
//				default:
//					if sendErr := server.sendEventWithRetry(agentStream.Stream, event, constants.MaxSendRetries, time.Second); sendErr != nil {
//						agentStream.ErrorChannel <- sendErr
//					}
//					wait.Done()
//				}
//			} else {
//				// broadcast event to other chat-service pods
//				eventWrapper := &chatService.EventWrapper{
//					AgentId: agentId.String(),
//					Event:   event,
//				}
//				eventWrapperJson, marshalErr := protojson.Marshal(eventWrapper)
//				if marshalErr != nil {
//					server.log.Logger.Error(serviceErrors.ErrJsonMarshal.Default,
//						zap.Error(marshalErr),
//						zap.String("agent id", agentId.String()))
//				} else {
//					rabbitUtil.PublishMessage(ctx, server.EventPublisher, constants.ChatEventExchanger, server.Consumer.RoutingKey, eventWrapperJson, server.log.Logger)
//				}
//			}
//		}
//	}
//}
//
//func (server *GRPCServer) sendEventWithRetry(stream chatService.ChatService_SubscribeToChatEventsNewServer, event *common.UserEvent, maxRetries int, delay time.Duration) error {
//	for i := 0; i < maxRetries; i++ {
//		if sendErr := stream.Send(event); sendErr != nil {
//			eventJson, _ := json.Marshal(event)
//			server.log.Logger.Error(serviceErrors.ErrSentUserEvent.Default,
//				zap.Error(sendErr),
//				zap.String("user event", string(eventJson)))
//
//			time.Sleep(delay)
//			continue
//		}
//		return nil
//	}
//	return status.Errorf(codes.Aborted, serviceErrors.ErrSentUserEvent.Default)
//}
