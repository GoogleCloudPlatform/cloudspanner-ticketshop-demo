/**
 * Copyright 2018 Google Inc. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

'use strict';

/** PubSub class. */
class PubSub {
    /**
     * PubSub initialization.
     * @constructor
     */
    constructor() {
        this.actions = [];
    }

    /**
     * Store a callback in an event
     * @param {string} e - A custom event name
     * @param {callback} cb - A callback function
     */
    on(e, cb) {
        if (!this.actions[e]) {
            this.actions[e] = [cb];
        } else {
            this.actions[e].push(cb);
        }
    }

    /**
     * Remove a callback from an event
     * @param {string} e - A custom event name
     * @param {callback} [cb] - An optional callback function
     */
    off(e, cb) {
        if (this.actions[e]) {
            if (!cb) {
                delete this.actions[e];
            } else {
                this.actions[e].forEach((fn, i) => {
                    if (fn === cb) {
                        this.actions[e].splice(i, 1);
                    }
                });
            }
        }
    }

    /**
     * Trigger an event
     * @param {string} e - A custom event name
     * @param {Object} [d] - Optional data packet
     */
    trigger(e, d) {
        const data = d || {};
        if (this.actions[e]) {
            this.actions[e].forEach((cb) => cb(data));
        }
    }
}

// Single export
const PS = new PubSub();