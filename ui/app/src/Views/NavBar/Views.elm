module Views.NavBar.Views exposing (navBar)

import Html exposing (Html, header, text, a, nav, ul, li)
import Html.Attributes exposing (class, href, title, style)
import Types exposing (Route(..))
import Views.NavBar.Types exposing (Tab, alertsTab, silencesTab, statusTab, noneTab, tabs)


navBar : Route -> Html msg
navBar currentRoute =
    header
        [ class "navbar navbar-toggleable-md navbar-light "
        , style [ ( "margin-bottom", "3rem" ), ( "border-bottom", "1px solid rgba(0,0,0,.15)" ) ]
        ]
        [ nav [ class "container" ]
            [ a [ class "navbar-brand", href "#" ] [ text "AlertManager" ]
            , ul [ class "navbar-nav" ] (navBarItems currentRoute)
            ]
        ]


navBarItems : Route -> List (Html msg)
navBarItems currentRoute =
    List.map (navBarItem currentRoute) tabs


navBarItem : Route -> Tab -> Html msg
navBarItem currentRoute tab =
    li [ class <| "nav-item" ++ (isActive currentRoute tab) ]
        [ a [ class "nav-link", href tab.link, title tab.name ]
            [ text tab.name ]
        ]


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

        SilenceRoute _ ->
            silencesTab

        StatusRoute ->
            statusTab

        TopLevelRoute ->
            noneTab
