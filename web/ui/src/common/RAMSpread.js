/* jshint esnext: true */

/*****************************************************************************
 * 
 * Copyright (C) Zenoss, Inc. 2015, all rights reserved.
 * 
 * This content is made available according to terms specified in
 * License.zenoss under the directory where your Zenoss product is installed.
 * 
 ****************************************************************************/

(function(){
    "use strict";

    var ramSpread = angular.module("ramSpread", []);

    var deps = ["$scope"];
    class RAMSpreadController{
        constructor($scope){
        }

        isOvercommited(){
            return this.last > this.commitment;
        }
        isCalculating(){
            // if there is some commitment, but we do not
            // yet have last, max and average, then this
            // is still calculating
            return this.hasCommitment() &&
                (this.last === null ||
                 this.max === null ||
                 this.average === null);
        }
        hasCommitment(){
            return this.commitment !== 0;
        }
    }
    deps.push(RAMSpreadController);
    ramSpread.controller("RAMSpreadController", deps);

    ramSpread.directive("ramSpread", function(){
        return {
            restrict: "E",
            scope: {
                last: "=",
                max: "=",
                average: "=",
                commitment: "="
            },
            controller: "RAMSpreadController",
            controllerAs: "vm",
            bindToController: true,
            template: `
                <span ng-show="vm.isCalculating()">Calculating...</span>
                <span ng-show="!vm.isCalculating()" ng-class="{'bad': vm.isOvercommited()}" class="overcomText">
                    {{vm.last|toMB:true}} / {{vm.max|toMB:true}} / {{vm.average|toMB:true}} MB
                </span>
            `
        };
    });
})();
