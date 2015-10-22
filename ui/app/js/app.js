'use strict';

angular.module('am.directives', []);

angular.module('am.directives').directive('route',
  function(RecursionHelper) {
    return {
      restrict: 'E',
      scope: {
        route: '='
      },
      templateUrl: '/app/partials/route.html',
      compile: function(element) {
        // Use the compile function from the RecursionHelper,
        // And return the linking function(s) which it returns
        return RecursionHelper.compile(element);
      }
    };
  }
);

angular.module('am.directives').directive('alert',
  function() {
    return {
      restrict: 'E',
      scope: {
        a: '='
      },
      templateUrl: '/app/partials/alert.html'
    };
  }
);

angular.module('am.services', ['ngResource']);

angular.module('am.services').factory('RecursionHelper',
  function($compile) {
    return {
      /**
       * Manually compiles the element, fixing the recursion loop.
       * @param element
       * @param [link] A post-link function, or an object with function(s) registered via pre and post properties.
       * @returns An object containing the linking functions.
       */
      compile: function(element, link) {
        // Normalize the link parameter
        if (angular.isFunction(link)) {
          link = {
            post: link
          };
        }

        // Break the recursion loop by removing the contents
        var contents = element.contents().remove();
        var compiledContents;
        return {
          pre: (link && link.pre) ? link.pre : null,
          /**
           * Compiles and re-adds the contents
           */
          post: function(scope, element) {
            // Compile the contents
            if (!compiledContents) {
              compiledContents = $compile(contents);
            }
            // Re-add the compiled contents to the element
            compiledContents(scope, function(clone) {
              element.append(clone);
            });

            // Call the post-linking function, if any
            if (link && link.post) {
              link.post.apply(null, arguments);
            }
          }
        };
      }
    };
  }
);

angular.module('am.services').factory('Silence',
  function($resource) {
    return $resource('', {
      id: '@id'
    }, {
      'query': {
        method: 'GET',
        url: '/api/v1/silences'
      },
      'create': {
        method: 'POST',
        url: '/api/v1/silences'
      },
      'get': {
        method: 'GET',
        url: '/api/v1/silence/:id'
      },
      'save': {
        method: 'POST',
        url: '/api/v1/silence/:id'
      },
      'delete': {
        method: 'DELETE',
        url: '/api/v1/silence/:id'
      }
    });
  }
);

angular.module('am.services').factory('Alert',
  function($resource) {
    return $resource('', {}, {
      'query': {
        method: 'GET',
        url: '/api/v1/alerts'
      }
    });
  }
);

angular.module('am.services').factory('Route',
  function($resource) {
    return $resource('', {}, {
      'query': {
        method: 'GET',
        url: '/api/v1/routes',
        params: {
          'pruneEmpty': 'true'
        }
      }
    });
  }
);

angular.module('am.services').factory('Alert',
  function($resource) {
    return $resource('', {}, {
      'query': {
        method: 'GET',
        url: '/api/v1/alerts'
      }
    });
  }
);
angular.module('am.controllers', []);

angular.module('am.controllers').controller('NavCtrl',
  function($scope, $location) {
    $scope.items = [{
      name: 'Silences',
      url: '/silences'
    }, {
      name: 'Alerts',
      url: '/alerts'
    }, {
      name: 'Status',
      url: '/status'
    }];

    $scope.selected = function(item) {
      return item.url == $location.path()
    }
  }
);

angular.module('am.controllers').controller('AlertCtrl',
  function() {}
);

angular.module('am.controllers').controller('AlertsCtrl',
  function($scope, Route) {
    $scope.route = null;
    $scope.order = "startsAt";

    $scope.refresh = function() {
      Route.query({},
        function(data) {
          console.log(data);
          $scope.route = data.data;
          console.log($scope.route)
        },
        function(data) {

        }
      );
    }

    $scope.refresh();
  }
);

// angular.module('am.controllers').controller('AlertsCtrl',
//   function($scope, Alert) {
//     $scope.alerts = [];
//     $scope.order = "startsAt";

//     $scope.refresh = function() {
//       Alert.query({},
//         function(data) {
//           $scope.alerts = data.data || [];
//           console.log($scope.alerts)
//         },
//         function(data) {

//         }
//       );
//     }

//     $scope.refresh();
//   }
// );

angular.module('am.controllers').controller('SilencesCtrl',
  function($scope, Silence) {
    $scope.silences = [];
    $scope.order = "startsAt";

    $scope.refresh = function() {
      Silence.query({},
        function(data) {
          $scope.silences = data.data || [];
        },
        function(data) {

        }
      );
    }

    $scope.delete = function(sil) {
      Silence.delete({
        id: sil.id
      })
      $scope.refresh()
    }

    $scope.refresh();
  }
);

angular.module('am.controllers').controller('SilenceCreateCtrl',
  function($scope, Silence) {

    $scope.create = function() {
      Silence.create($scope.silence,
        function(data) {
          $scope.refresh();
        },
        function(data) {
          $scope.error = data.data;
        }
      );
    }

    $scope.error = null;

    $scope.newMatcher = function() {
      $scope.silence.matchers.push({});
    }
    $scope.delMatcher = function(i) {
      $scope.silence.matchers.splice(i, 1);
    }
    $scope.reset = function() {
      var now = new Date(),
        end = new Date();

      now.setMilliseconds(0);
      end.setMilliseconds(0);
      now.setSeconds(0);
      end.setSeconds(0);

      end.setHours(end.getHours() + 2)

      $scope.silence = {
        startsAt: now,
        endsAt: end,
        matchers: [{}],
        comment: "",
        createdBy: ""
      }
    }

    $scope.reset();
  }
);

angular.module('am', [
  'ngRoute',

  'am.controllers',
  'am.services',
  'am.directives'
]);

angular.module('am').config(
  function($routeProvider) {
    $routeProvider.
    when('/alerts', {
      templateUrl: '/app/partials/alerts.html',
      controller: 'AlertsCtrl'
    }).
    when('/silences', {
      templateUrl: '/app/partials/silences.html',
      controller: 'SilencesCtrl'
    }).
    otherwise({
      redirectTo: '/silences'
    });
  }
);