/*
 * baseFactory.js
 * baseFactory constructs a factory object that can be used
 * to keep the UI in sync with the backend. The returned factory
 * will use the provided update function (which should return a
 * promise good for an map of id to object), create new objects using
 * the provided ObjConstructor, and cache those objects.
 *
 * When it hits the update function again, it will compare the new
 * list to the cached list and intelligently new, update, and
 * remove objects as needed.
 *
 * NOTE: you can override update, mixin methods, etc to make this
 * thing more useful
 */

(function() {
    'use strict';

    const DEFAULT_UPDATE_FREQUENCY = 3000;
    const DEFAULT_UPDATE_TIMEOUT = 6000;

    var $q, $timeout, $rootScope;

    angular.module('baseFactory', []).
    factory("baseFactory", ["$q", "$timeout", "$rootScope", "servicedConfig",
    function(_$q, _$timeout, _$rootScope, servicedConfig){
        $q = _$q;
        $timeout = _$timeout;
        $rootScope = _$rootScope;

        servicedConfig.getConfig()
            .then(config => {
                this.updateFrequency = config.PollFrequency * 1000;
            }).catch(err => {
                let errMessage = err.statusText;
                if(err.data && err.data.Detail){
                    errMessage = err.data.Detail;
                }
                console.error("could not load serviced config:", errMessage);
            });

        return BaseFactory;
    }]);

    // BaseFactory creates and returns a new factory/cache
    // @param {function} ObjConstructor - constructor function to be new'd up
    //      with each object from the backend. NOTE: ObjConstructor must provide
    //      update and updateObjDef methods.
    // @param {function} updateFn - function to be called to update the object
    //      cache. NOTE: this function must return a promise that yields a map
    //      of id to object.
    function BaseFactory(ObjConstructor, updateFn){
        // map of cached objects by id
        this.objMap = {};
        // array of cached objects
        this.objArr = [];
        this.updateFn = updateFn;
        this.ObjConstructor = ObjConstructor;

        this.updateFrequency = DEFAULT_UPDATE_FREQUENCY;
        this.updateTimeout = DEFAULT_UPDATE_TIMEOUT;
        this.shouldUpdate = false;
    }


    BaseFactory.prototype = {
        constructor: BaseFactory,

        // TODO - debounce
        // update calls the provided update function, iterates the results,
        // compares them to cached results and updates, creates, or deletes
        // objects based on id
        update: function(){
            var deferred = $q.defer();
            this.updateFn()
                .success((data, status) => {
                    var included = [];

                    for(let id in data){
                        let obj = data[id];

                        // update
                        if(this.objMap[id]){
                            this.objMap[id].update(obj);

                        // new
                        } else {
                            this.objMap[id] = new this.ObjConstructor(obj);
                            this.objArr.push(this.objMap[id]);
                        }

                        included.push(id);
                    }

                    // delete
                    if(included.length !== Object.keys(this.objMap).length){
                        // iterate objMap and find keys
                        // not present in included list
                        for(let id in this.objMap){
                            if(included.indexOf(id) === -1){
                                this.objArr.splice(this.objArr.indexOf(this.objMap[id]), 1);
                                delete this.objMap[id];
                            }
                        }
                    }

                    deferred.resolve();
                })
                .error((data, status) => {
                    console.error("Unable to update factory", data);
                    deferred.reject("Unable to update factory", data);
                })
                .finally(() => {
                    // notify the first request is complete
                    if(!this.lastUpdate){
                        $rootScope.$emit("ready");
                    }

                    this.lastUpdate = new Date().getTime();
                });
            return deferred.promise;
        },

        // slowdown update timing. for when things go south
        backoffUpdateTimers: function(){
            // TODO - better algorithm
            this.updateFrequency *= 2;
            this.updateTimeout *= 2;
            // TODO - expose a way to notify user that backoff is occurring
        },

        // restores default update timing. when things are so nice again
        restoreUpdateTimers: function(){
            this.updateFrequency = DEFAULT_UPDATE_FREQUENCY;
            this.updateTimeout = DEFAULT_UPDATE_TIMEOUT;
        },

        // determines if updateManager should be run again,
        // and runs it if necessary
        updateManagerSchedule: function(){
            if(this.shouldUpdate){
                // cancel any pending update
                this.updateManagerCancel();
                this.updatePromise = $timeout(() => {
                    this.updateManagerRun();
                }, this.updateFrequency);
            }
        },

        // updates factory at predefined interval and backs
        // off if backend is responding slowly
        updateManagerRun: function(){
            let timeoutTimer;
            this.updateManagerCancel();
            this.update()
                .then(() => {
                    this.restoreUpdateTimers();
                    console.log("update success, restoring timers");
                },
                (err) => {
                    this.backoffUpdateTimers();
                    console.log("backing off timers to", this.updateFrequency, "because:", err);
                })
                .finally(() => {
                    // cancel timeout timer
                    $timeout.cancel(timeoutTimer);
                    this.updateManagerSchedule();
                });

            // ensure no request exceeds a certain timeout
            timeoutTimer = $timeout(() => {
                this.backoffUpdateTimers();
                console.log("backing off timers to", this.updateFrequency,
                    "because request timed out after", this.updateTimeout);
                this.updateManagerSchedule();
            }, this.updateTimeout);
        },

        updateManagerCancel: function(){
            if(this.updatePromise){
                $timeout.cancel(this.updatePromise);
            }
        },

        // begins auto-update
        activate: function(){
            this.shouldUpdate = true;
            this.updateManagerRun();
        },

        // stops auto-update
        deactivate: function(){
            this.shouldUpdate = false;
            this.updateManagerCancel();
        },

        // returns an object by id
        get: function(id){
            return this.objMap[id];
        }
    };


/*
 * example of a type that could be passed
 * in as ObjectConstructor

        function Obj(obj){
            this.update(obj);

            // do more init stuff here
        }

        Obj.prototype = {
            constructor: Obj,
            update: function(obj){
                // if obj is provided, update
                // immutable internal representation
                // of that object
                if(obj){
                    this.updateObjDef(obj);
                }

                // do more update stuff here
            },

            // update immutable copy of the object
            // from the backend
            updateObjDef: function(obj){
                // alias some properties for easy access
                this.name = obj.Name;
                this.id = obj.ID;
                this.model = Object.freeze(obj);

                // do more update stuff here
            },
        };
*/

})();
