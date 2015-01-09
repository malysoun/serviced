/* healthIconDirective
 * directive for displaying health of a service/instance
 * in icon form with helpful popover details
 */
(function() {
    'use strict';

    angular.module('healthIcon', [])
    .directive("healthIcon", [
    function(){
        return {
            restrict: "E",
            scope: {
                // status object generated by serviceHealth
                service: "="
            },
            template: '<i class="healthIcon glyphicon"></i><div class="healthIconBadge"></div>',
            link: function($scope, element, attrs){
                var STATUS_STYLES = {
                    "bad": "glyphicon glyphicon-exclamation bad",
                    "good": "glyphicon glyphicon-ok good",
                    "unknown": "glyphicon glyphicon-question unknown",
                    "down": "glyphicon glyphicon-minus disabled"
                };

                // cache some DOM elements
                var $el = $(element),
                    id = $el.attr("data-id"),
                    $healthIcon = $el.find(".healthIcon"),
                    $badge = $el.find(".healthIconBadge");

                function update(){
                    var lastStatus = $el.attr("data-lastStatus"),
                        statusObj, popoverHTML,
                        hideHealthChecks,
                        placement = "right",
                        popoverObj, template;

                    statusObj = $scope.service.status;

                    if(!statusObj){
                        console.log("no status obj");
                        return;
                    }

                    console.log("running icon update");

                    // this instance is on its way up, so create an "unknown" status for it
                    // TODO - implement this
                    /*
                    if(!statusObj){
                        statusObj = new Status(id, "", 1);
                        statusObj.statusRollup.incDown();
                        statusObj.evaluateStatus();
                    }*/

                    // determine if we should hide healthchecks
                    hideHealthChecks = statusObj.statusRollup.allGood() ||
                        statusObj.statusRollup.allDown() ||
                        statusObj.desiredState === 0;

                    // if service should be up and there is more than 1 instance, show number of instances
                    if(statusObj.desiredState === 1 && statusObj.statusRollup.total > 1){
                        $el.addClass("wide");
                        $badge.text(statusObj.statusRollup.good +"/"+ statusObj.statusRollup.total).show();

                    // else, hide the badge
                    } else {
                        $el.removeClass("wide");
                        $badge.hide();
                    }

                    // setup popover

                    // if this $el is inside a .serviceTitle, make the popover point down
                    if($el.parent().hasClass("serviceTitle")){
                        placement = "bottom";
                    }

                    // if this statusObj has children, we wanna show
                    // them in the healtcheck tooltip, so generate
                    // some yummy html
                    if(statusObj.children.length){
                        popoverHTML = [];

                        var isHealthCheckStatus = function(status){
                           return !status.id;
                        };

                        // if this status's children are healthchecks,
                        // no need for instance rows, go straight to healthcheck rows
                        if(statusObj.children.length && isHealthCheckStatus(statusObj.children[0])){
                            // if these are JUST healthchecks, then don't allow them
                            // to be hidden. this ensures that healthchecks show up for
                            // running instances.
                            hideHealthChecks = false;
                            // don't show count badge for healthchecks either
                            $badge.hide();
                            $el.removeClass("wide");

                            statusObj.children.forEach(function(hc){
                                popoverHTML.push(bindHealthCheckRowTemplate(hc));
                            });

                        // else these are instances, so create instance rows
                        // AND healthcheck rows
                        } else {
                            statusObj.children.forEach(function(instanceStatus){
                                // if this is becoming too long, stop adding rows
                                if(popoverHTML.length >= 15){
                                    // add an overflow indicator if not already there
                                    if(popoverHTML[popoverHTML.length-1] !== "..."){
                                        popoverHTML.push("..."); 
                                    }
                                    return;
                                }

                                // only create an instance row for this instance if
                                // it's in a bad or unknown state
                                if(instanceStatus.status === "bad" || instanceStatus.status === "unknown"){
                                    popoverHTML.push("<div class='healthTooltipDetailRow'>");
                                    popoverHTML.push("<div style='font-weight: bold; font-size: .9em; padding: 5px 0 3px 0;'>"+ instanceStatus.name +"</div>");
                                    instanceStatus.children.forEach(function(hc){
                                        popoverHTML.push(bindHealthCheckRowTemplate(hc));
                                    });
                                    popoverHTML.push("</div>");
                                }
                            });
                        }

                        popoverHTML = popoverHTML.join("");
                    }

                    // choose a popover template which is just a title,
                    // or a title and content
                    template = hideHealthChecks || !popoverHTML ?
                        '<div class="popover" role="tooltip"><div class="arrow"></div><h3 class="popover-title"></h3></div>' :
                        '<div class="popover" role="tooltip"><div class="arrow"></div><h3 class="popover-title"></h3><div class="popover-content"></div></div>';

                    // NOTE: directly accessing the bootstrap popover
                    // data object here.
                    popoverObj = $el.data("bs.popover");

                    // if popover element already exists, update it
                    if(popoverObj){
                        // update title, content, and template
                        popoverObj.options.title = statusObj.description;
                        popoverObj.options.content = popoverHTML;
                        popoverObj.options.template = template;

                        // if the tooltip already exists, change the contents
                        // to the new template
                        if(popoverObj.$tip){
                            popoverObj.$tip.html($(template).html());
                        }

                        // force popover to update using the new options
                        popoverObj.setContent();

                        // if the popover is currently visible, update
                        // it immediately, but turn off animation to
                        // prevent it fading in
                        if(popoverObj.$tip.is(":visible")){
                            popoverObj.options.animation = false;
                            popoverObj.show();
                            popoverObj.options.animation = true;
                        }

                    // if popover element doesn't exist, create it
                    } else {
                        $el.popover({
                            trigger: "hover",
                            placement: placement,
                            delay: 0,
                            title: statusObj.description,
                            html: true,
                            content: popoverHTML,
                            template: template
                        });
                    }

                    $el.removeClass(Object.keys(STATUS_STYLES).join(" "))
                        .addClass(statusObj.status);

                    // if the status has changed since last tick, or
                    // it was and is still unknown, notify user
                    if(lastStatus !== statusObj.status || lastStatus === "unknown" && statusObj.status === "unknown"){
                        bounceStatus($el);
                    }
                    // store the status for comparison later
                    $el.attr("data-lastStatus", statusObj.status);
                }

                // if status object updates, update icon
                $scope.$watch("service.status", update);

                // TODO - cleanup watch
                $scope.$on("$destroy", function(){});
                
                function bindHealthCheckRowTemplate(hc){
                    return "<div class='healthTooltipDetailRow "+ hc.status +"'>\
                            <i class='healthIcon glyphicon'></i>\
                        <div class='healthTooltipDetailName'>"+ hc.name +"</div>\
                    </div>";
                }

                function bounceStatus($el){
                    $el.addClass("zoom");

                    $el.on("webkitAnimationEnd animationend", function(){
                        $el.removeClass("zoom");
                        // clean up animation end listener
                        $el.off("webkitAnimationEnd animationend");
                    });
                }

            }
        };
    }]);
})();
