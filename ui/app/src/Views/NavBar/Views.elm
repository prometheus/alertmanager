module Views.NavBar.Views exposing (appHeader)

import Html exposing (Html, header, text, a, nav)
import Html.Attributes exposing (class, href, title)


appHeader : List ( String, String ) -> Html msg
appHeader links =
    let
        headerLinks =
            List.map (\( link, text ) -> headerLink link text) links
    in
        header [ class "bg-black-90 fixed w-100 ph3 pv3 pv4-ns ph4-m ph5-l" ]
            [ nav [ class "w-80 center f6 fw6 ttu tracked" ]
                headerLinks
            ]


headerLink : String -> String -> Html msg
headerLink link linkText =
    a [ class "link dim white dib mr3", href link, title linkText ]
        [ text linkText ]
