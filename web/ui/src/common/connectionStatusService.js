
/* connectionStatusService.js
 * monitors responses from serviced and alerts user
 * if responses are slowing
 */
(function(){
    "use strict";

    let $rootScope, $notification;

    // find and remove an element from
    // an array. returns either the
    // element or `null` if element isnt found
    function removeElement(arr, el){
        let i = arr.indexOf(el);
        if(i !== -1){
            return arr.splice(i, 1);
        }
        return null;
    }

    class ConnectionStatus {
        constructor(){
            // a registeredService is a wrapper around a service, that
            // includes the event listener de-registrations
            this.registeredServices = [];
            // timingOut services are just the regular service object,
            // not a wrapper as is the case with registeredServices
            this.timingOutServices = [];
            this.notification = null;
        }

        registerServices(...services){
            services.forEach(service => {
                this.registerService(service);
            });
        }
        registerService(service){
            let registeredService = {
                service: service,
                deregistrations: [
                    $rootScope.$on("backoff.thresholdReached", (e, ctx) => {
                        this.onBackoffThresholdReached(ctx);
                    }),
                    $rootScope.$on("backoff.reset", (e, ctx) => {
                        this.onBackoffReset(ctx);
                    })
                ]
            };
            this.registeredServices.push(registeredService);
        }

        unregisterServices(...services){
            services.forEach(service => {
                this.unregisterService(service);
            });
        }
        unregisterService(service){
            // search the list of registered services for
            // the service in question
            let index = this.registeredServices.map(rs => rs.service).indexOf(service);

            if(index !== -1){
                let registeredService = this.registeredServices[index];
                registeredService.deregistrations.forEach(fn => fn());
                this.registeredServices.splice(index, 1);

                // remove this service from timing out services
                removeElement(this.timingOutServices, service);
            }
        }

        onBackoffThresholdReached(service){
            // if no notification is up, add one
            if(!this.notification){
                this.notification = $notification.create("Connection Problems", "Serviced is currently slow to respond");
                this.notification.warning(false);
            }
            // add this service to list of timing out services
            if(this.timingOutServices.indexOf(service) === -1){
                this.timingOutServices.push(service);
                console.log("timingOutServices.length:", this.timingOutServices.length);
            }
        }
        onBackoffReset(service){
            // remove this service from timing out services
            removeElement(this.timingOutServices, service);

            // if a notification is up and no more services
            // are timing out, clear the notification
            if(this.notification && !this.timingOutServices.length){
                this.notification.hide();
                this.notification = null;
                $notification.create("Connection Restored", "").info();
                console.log("timingOutServices.length:", this.timingOutServices.length);
            }
        }

        // clear any services and notifications
        destroy(){
            this.unregisterServices(this.registeredServices.map(registered => registered.service));
            this.registeredServices = [];
            this.timingOutServices = [];
            if(this.notification){
                this.notification.hide();
                this.notification = null;
            }
        }

    }

    var connectionStatus = new ConnectionStatus();

    angular.module("connectionStatus", [])
    .factory("connectionStatus", ["$rootScope", "$notification",
    function(_$rootScope, _$notification){
        $rootScope = _$rootScope;
        $notification = _$notification;
        return connectionStatus;
    }]);

})();
