package clientpkg

import (
	"context"
	"encoding/json"

	"github.com/Seann-Moser/mserve"
)

type Iterator[T any] struct {
	ctx         context.Context
	err         error
	client      HttpClient
	current     *T
	currentItem int
	currentPage uint

	totalItems   int
	offset       int
	currentPages []*T
	singlePage   bool
	retry        bool
	RequestData  RequestData
	message      string
}

func NewIterator[T any](ctx context.Context, client HttpClient, data RequestData) *Iterator[T] {
	it := &Iterator[T]{
		ctx:          ctx,
		client:       client,
		currentPages: make([]*T, 0),
		RequestData:  data,
	}
	it.getPages()
	return it
}

func (i *Iterator[T]) WithRetry() *Iterator[T] {
	i.retry = true
	return i
}
func (i *Iterator[T]) Current() *T {
	if i.current == nil {
		if len(i.currentPages) == 0 {
			if !i.getPages() {
				return nil
			}
		}
		if i.currentItem-i.offset >= len(i.currentPages) {
			return nil
		}
		i.current = i.currentPages[i.currentItem-i.offset]
	}
	return i.current
}

func (i *Iterator[T]) Message() string {
	return i.message
}

func (i *Iterator[T]) Err() error {
	return i.err
}

func (i *Iterator[T]) FullList() ([]*T, error) {
	var fullList []*T
	fullList = append(fullList, i.currentPages...)
	for i.Next() {
		current := i.Current()
		if current != nil {
			fullList = append(fullList, current)
		}
	}
	if i.Err() != nil {
		return nil, i.Err()
	}
	return fullList, nil
}

func (i *Iterator[T]) Next() bool {
	if i.singlePage {
		return false
	}
	//todo fix
	if i.totalItems == 0 {
		if !i.getPages() {
			return false
		}
		if len(i.currentPages) == 0 {
			return false
		}
		i.current = i.currentPages[i.currentItem-i.offset]
		return true
	}
	if i.currentItem < i.totalItems {
		i.currentItem += 1
		if i.currentItem-i.offset >= len(i.currentPages) {
			if !i.getPages() {
				return false
			}
		}
		if i.currentItem-i.offset >= len(i.currentPages) {
			return false
		}
		i.current = i.currentPages[i.currentItem-i.offset]
		return true
	}
	return false
}

// todo support cookies
func (i *Iterator[T]) getPages() bool {
	data := i.client.Request(i.ctx, i.RequestData, i.nextPage(), i.retry)
	if data.Err != nil {
		i.err = data.Err
		i.message = data.Message
		return false
	} else {
		i.message = data.Message
		if len(data.Data) == 0 {
			return false
		}
		i.err = json.Unmarshal(data.Data, &i.currentPages)
		if i.err != nil {
			var single T
			//logic to read single response
			tmpErr := json.Unmarshal(data.Data, &single)
			if tmpErr != nil {
				return false
			}
			i.err = nil
			i.singlePage = true
			i.currentPages = []*T{&single}
			return true
		}
		i.totalItems = int(data.Page.Total)

		i.offset = int((data.Page.Page - 1) * data.Page.Limit)
	}
	if i.err != nil {
		return false
	}
	return true
}

func (i *Iterator[T]) nextPage() *mserve.Pagination {
	if i.currentPage <= 0 {
		i.currentPage = 1
	}
	page := &mserve.Pagination{
		Page: int(i.currentPage),
	}
	i.currentPage += 1
	return page
}
