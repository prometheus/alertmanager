module Views.NavBar.Views exposing (navBar)

import Html exposing (Html, a, div, header, li, nav, text, ul)
import Html.Attributes exposing (attribute, class, href, style, target, title)
import String exposing (startsWith)
import Types exposing (Route(..))
import Views.NavBar.Types exposing (Tab(..), alertsTab, noneTab, silencesTab, statusTab, tabs)


navBar : Route -> Html msg
navBar currentRoute =
    header
        [ class "navbar navbar-toggleable-md navbar-light bg-faded mb-5 pt-3 pb-3"
        , style "border-bottom" "1px solid rgba(0, 0, 0, .125)"
        ]
        [ nav [ class "container" ]
            [ a [ class "navbar-brand", href "#" ] [ text "Alertmanager" ]
            , ul [ class "navbar-nav" ] (navBarItems currentRoute)
            , case currentRoute of
                SilenceFormEditRoute _ ->
                    text ""

                SilenceFormNewRoute _ ->
                    text ""

                _ ->
                    div [ class "form-inline ml-auto" ]
                        [ a
                            [ class "btn btn-outline-info"
                            , href "#/silences/new"
                            ]
                            [ text "New Silence" ]
                        ]
            ]
        ]


navBarItems : Route -> List (Html msg)
navBarItems currentRoute =
    List.map (navBarItem currentRoute) tabs


navBarItem : Route -> Tab -> Html msg
navBarItem currentRoute tab =
    case tab of
        ST st ->
            li [ class <| "nav-item" ++ isActive currentRoute tab ]
                [ a [ class "nav-link", href st.link, title st.name, target (linkToTarget st.link) ]
                    [ text st.name ]
                ]

        DT dt ->
            li [ class <| "dropdown" ]
                [ a [ target "_blank", class "nav-link dropdown-toggle", attribute "data-toggle" "dropdown", attribute "role" "button", href "#", attribute "aria-haspopup" "true", attribute "aria-expanded" "false" ]
                    [ text dt.name ]
                , ul [ class "dropdown-menu" ] (List.map (navBarItem currentRoute) dt.lTab)
                ]


linkToTarget : String -> String
linkToTarget link =
    if startsWith "http" link then
        "_blank"

    else
        ""


isActive : Route -> Tab -> String
isActive currentRoute tab =
    if routeToTab currentRoute == tab then
        " active"

    else
        ""


routeToTab : Route -> Tab
routeToTab currentRoute =
    case currentRoute of
        AlertsRoute _ ->
            alertsTab

        NotFoundRoute ->
            noneTab

        SilenceFormEditRoute _ ->
            silencesTab

        SilenceFormNewRoute _ ->
            silencesTab

        SilenceListRoute _ ->
            silencesTab

        SilenceViewRoute _ ->
            silencesTab

        StatusRoute ->
            statusTab

        TopLevelRoute ->
            noneTab
