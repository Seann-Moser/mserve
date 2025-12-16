export class Iterator {
    // todo look up better way of doing an async iterator in js
    constructor(data, path, config, page) {
        this.config = config;
        this.path = path;
        this.loading = false;
        if (data !== null) {
            this.responseData = new IteratorResponseData(data);
        } else {
            this.responseData = null;
        }

        this.currentPages = [];
        this.current = null;
        this.err = null;
        this.message = "";

        this.offset = 0;
        this.singlePage = false;
        this.currentItem = 0;
        if (page !== null) {
            this.pagination = page;
        } else {
            this.pagination = new Pagination(null);
        }
    }

    /**
     *
     * @param {int} itemsPerPage
     * @constructor
     */
    SetItemsPerPage(itemsPerPage) {
        if (itemsPerPage > 500 || itemsPerPage < 1) {
            return;
        }
        this.pagination.ItemsPerPage = itemsPerPage;
        return new Promise((resolve) => resolve(this.currentPages));
    }

    async GetCurrent() {
        if (this.current == null) {
            if (this.currentPages === null || this.currentPages.length === 0) {
                const v = await this.getPages();
                if (!v) {
                    return Promise.reject(
                        createError(`failed loading pages ${this.err}`),
                    );
                }

                if (!Array.isArray(this.currentPages)) {
                    this.singlePage = true;
                    this.current = this.currentPages;
                    return new Promise((resolve) => resolve(this.current));
                }
            }
            if (this.currentItem - this.offset >= this.currentPages.length) {
                return Promise.reject(
                    createError(
                        `index out of bounds ${this.currentItem - this.offset} > ${
                            this.currentPages.length
                        }`,
                    ),
                );
            }
            this.current = this.currentPages[this.currentItem - this.offset];
        }
        return new Promise((resolve) => resolve(this.current));
    }

    /**
     * @return {promise<array>|promise<null>}
     */
    async GetPage() {
        if (
            this.currentPages === null ||
            this.currentPages.length === 0 ||
            this.currentPages === undefined
        ) {
            const v = await this.getPages();
            if (!v) {
                return new Promise((resolve) => resolve(null));
            }
        }
        if (this.currentPages === undefined) {
            return new Promise((resolve) => resolve(null));
        }
        if (!Array.isArray(this.currentPages)) {
            this.singlePage = true;
            this.current = this.currentPages;
            return new Promise((resolve) => resolve(this.current));
        }
        return new Promise((resolve) => resolve(this.currentPages));
    }

    /**
     * @param {int} pageNumber
     * @returns {promise<array>}
     * @constructor
     */
    async GoToPage(pageNumber) {
        if (this.singlePage) {
            return null;
        }
        this.pagination.CurrentPage = pageNumber;
        if (this.pagination.CurrentPage < 1) {
            this.pagination.CurrentPage = 1;
        }
        if (this.pagination.CurrentPage > this.pagination.TotalPages) {
            this.pagination.CurrentPage = this.pagination.TotalPages;
        }
        const v = await this.getPages();
        if (!v) {
            return Promise.reject(createError(`failed loading pages ${this.err}`));
        }
        return new Promise((resolve) => resolve(this.currentPages));
    }

    /**
     *
     * @return {promise<array>}
     * @constructor
     */
    async PreviousPage() {
        if (this.singlePage) {
            return null;
        }
        this.pagination.CurrentPage -= 1;
        if (this.pagination.CurrentPage < 1) {
            this.pagination.CurrentPage = 1;
        }
        const v = await this.getPages();
        if (!v) {
            return Promise.reject(createError(`failed loading pages ${this.err}`));
        }
        return new Promise((resolve) => resolve(this.currentPages));
    }

    /**
     *
     * @return {promise<array>}
     * @constructor
     */
    async NextPage() {
        if (this.singlePage) {
            return null;
        }
        this.pagination.CurrentPage += 1;
        if (this.pagination.CurrentPage < 1) {
            this.pagination.CurrentPage = 1;
        }
        if (this.pagination.CurrentPage > this.pagination.TotalPages) {
            this.pagination.CurrentPage = this.pagination.TotalPages;
        }
        const v = await this.getPages();
        if (!v) {
            return Promise.reject(
                createError(`failed getting next page: ${this.err}`),
            );
        }
        return new Promise((resolve) => resolve(this.currentPages));
    }

    GetPagination() {
        return this.pagination;
    }

    /**
     *
     * @returns {promise}
     */
    async Next() {
        if (this.singlePage) {
            return null;
        }
        if (this.pagination.TotalItems === 0) {
            const v = await this.getPages();
            if (!v) {
                return Promise.reject(
                    createError(`failed getting next page: ${this.err}`),
                );
            }
            if (!Array.isArray(this.currentPages)) {
                this.singlePage = true;
                this.current = this.currentPages;
                return new Promise((resolve) => resolve(this.current));
            }
            // todo check if it an array
            if (this.currentPages.length === 0) {
                return null;
            }
            this.current = this.currentPages[this.currentItem - this.offset];
            return new Promise((resolve) => resolve(this.current));
        }
        if (this.currentItem < this.pagination.TotalItems - 1) {
            this.currentItem += 1;
            if (this.currentItem - this.offset >= this.currentPages.length) {
                this.pagination.CurrentPage += 1; // todo fix
                const v = await this.getPages();
                if (!v) {
                    return Promise.reject(
                        createError(`failed loading pages ${this.err}`),
                    );
                }
            }
            if (!this.currentPages) {
                return Promise.reject(
                    createError(`failed loading page, current page is null`),
                );
            }
            if (this.currentItem - this.offset >= this.currentPages.length) {
                return null;
            }
            this.current = this.currentPages[this.currentItem - this.offset];
            return new Promise((resolve) => resolve(this.current));
        }
        return null;
    }

    /**
     *
     * @returns {promise}
     */
    async Previous() {
        if (this.singlePage) {
            return null;
        }
        if (this.pagination.TotalItems === 0) {
            const v = await this.getPages();
            if (!v) {
                return Promise.reject(
                    createError(`failed getting next page: ${this.err}`),
                );
            }
            if (!Array.isArray(this.currentPages)) {
                this.singlePage = true;
                this.current = this.currentPages;
                return new Promise((resolve) => resolve(this.current));
            }
            // todo check if it an array
            if (this.currentPages.length === 0) {
                return null;
            }
            this.current = this.currentPages[this.currentItem - this.offset];
            return new Promise((resolve) => resolve(this.current));
        }
        if (this.currentItem > 0) {
            this.currentItem -= 1;
            if (this.currentItem - this.offset < 0) {
                this.pagination.CurrentPage -= 1; // todo fix
                const v = await this.getPages();
                if (!v) {
                    return Promise.reject(
                        createError(`failed loading pages ${this.err}`),
                    );
                }
            }
            if (this.currentItem - this.offset < 0) {
                return null;
            }
            this.current = this.currentPages[this.currentItem - this.offset];
            return new Promise((resolve) => resolve(this.current));
        }
        return null;
    }

    Err() {
        return this.err;
    }

    /**
     *
     * @returns {promise<string>}
     * @constructor
     */
    async Message() {
        if (this.message === null || this.message === "") {
            if (this.responseData !== null) {
                return new Promise((resolve) => resolve(this.responseData.Message));
            } else {
                const v = await this.getPages();
                if (!v) {
                    return Promise.reject(
                        createError(`failed loading pages ${this.err}`),
                    );
                }
                return this.responseData.Message;
            }
        }
        return new Promise((resolve) => resolve(this.message));
    }

    /**
     *
     * @returns {promise<boolean>}
     */
    async getPages() {
        if (
            this.responseData &&
            this.responseData.rawResponse &&
            !this.responseData.Data
        ) {
            await this.responseData.LoadData();
            if (
                !(
                    this.responseData.Page === undefined ||
                    this.responseData.Page === null
                )
            ) {
                this.pagination = this.responseData.Page;
            }
            this.message = this.responseData.Message;
            this.currentPages = this.responseData.Data;

            if (!Array.isArray(this.currentPages)) {
                this.singlePage = true;
                this.current = this.currentPages;
                return new Promise((resolve) => resolve(true));
            }
            this.offset =
                (this.pagination.CurrentPage - 1) * this.pagination.ItemsPerPage;
        }
        if (
            this.responseData.Data === undefined ||
            this.responseData.Data === null ||
            this.currentItem - this.offset >= this.currentPages.length ||
            this.currentItem - this.offset < 0
        ) {
            this.config.params.items_per_page = this.pagination.ItemsPerPage;
            this.config.params.page = this.pagination.CurrentPage;
            try {
                this.loading = true;
                const data = await $fetch(this.path, this.config);
                this.responseData = new IteratorResponseData(data);

                await this.responseData.LoadData();
            } catch (error) {
                this.loading = false;
                this.err = error;
                this.message = error.Message;
                let message = "";
                console.error(error);
                try {
                    message = JSON.parse(error.data).message;
                } catch (_) {
                    message = error;
                }
                return new Promise(
                    (resolve) => resolve(false),
                    (reject) => reject(message),
                );
            }
            this.loading = false;
        }
        if (
            !(this.responseData.Page === undefined || this.responseData.Page === null)
        ) {
            this.pagination = this.responseData.Page;
        }
        this.message = this.responseData.Message;
        this.currentPages = this.responseData.Data;

        if (!Array.isArray(this.currentPages)) {
            this.singlePage = true;
            this.current = this.currentPages;
            return new Promise((resolve) => resolve(true));
        }
        this.offset =
            (this.pagination.CurrentPage - 1) * this.pagination.ItemsPerPage;
        return new Promise((resolve) => resolve(true));
    }
}

export class IteratorResponseData {
    constructor(rawResponse) {
        this.rawResponse = rawResponse;
        this.decoded = {};
        if (rawResponse === undefined) {
            return;
        }
        if (typeof rawResponse !== "object") {
            return;
        }
        if ("data" in rawResponse) {
            this.Data = rawResponse.data;
        } else {
            this.Data = null;
        }
        if ("page" in rawResponse) {
            this.Page = new Pagination(rawResponse.page);
        }

        if ("message" in rawResponse) {
            this.Message = rawResponse.message;
        }
    }

    async LoadData() {
        if (this.rawResponse === undefined) {
            return;
        }

        const rawData = await this.rawResponse;
        this.decoded = JSON.parse(rawData);
        this.parse();
    }

    parse() {
        if (typeof this.decoded !== "object") {
            return;
        }
        if ("data" in this.decoded) {
            this.Data = this.decoded.data;
        }
        if ("page" in this.decoded) {
            this.Page = new Pagination(this.decoded.page);
        }
        if ("message" in this.decoded) {
            this.Message = this.decoded.message;
        }
    }
}

export class Pagination {
    constructor(pageJson) {
        if (pageJson === null || pageJson === undefined) {
            this.CurrentPage = 1;
            this.ItemsPerPage = 24;
            return;
        }

        this.CurrentPage = pageJson.current_page;
        this.NextPage = pageJson.next_page;
        this.TotalItems = pageJson.total_items;
        this.TotalPages = pageJson.total_pages;
        this.ItemsPerPage = pageJson.items_per_page;
        if (this.CurrentPage <= 0) {
            this.CurrentPage = 1;
        }
        if (this.TotalPages <= 0) {
            this.TotalPages = 1;
        }
    }
}

/**
 @param {array<File>} files
 @param {object} config
 @return {promise}
 */
export function UploadImage(files, config) {
    if (files === null || files.length === 0) {
        return Promise.reject(
            createError(`failed uploading image:no files present`),
        );
    }
    const formData = new FormData();
    formData.append("image", files[0]);
    config.body = formData;

    return $fetch(config.path, config);
}
